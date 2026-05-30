package gateway

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
)

const (
	ReadTimeout       = 90 * time.Second // 读超时：90s 无数据则断开
	WriteTimeout      = 30 * time.Second // 写超时：单次发送最大等待 30s
	HeartbeatInterval = 30 * time.Second // 心跳检测间隔：每 30s 检查一次在线
	SendQueueSize     = 1024             // 发送消息队列最大缓冲数
)

type QUICSession struct {
	conn         *quic.Conn       // QUIC 底层连接对象
	uid          string           // 当前登录用户ID
	authDone     chan struct{}    // 鉴权完成信号通道
	done         chan struct{}    // 会话关闭信号通道（全局退出标记）
	sendQueue    chan []byte      // 待下发消息队列
	lastActive   time.Time        // 最后一次活跃时间（用于心跳/超时）
	readTimeout  time.Duration    // 读超时配置
	writeTimeout time.Duration    // 写超时配置
	closeOnce    sync.Once        // 保证 Close 只执行一次
	authOnce     sync.Once        // 保证鉴权完成信号只发送一次
	mu           sync.RWMutex     // 读写锁：保护 uid / lastActive / onTouch
	onTouch      func(uid string) // 客户端活跃回调（外部注入，用来刷新全局在线状态）
}

func NewQUICSession(conn *quic.Conn, queueSize int, readTimeout, writeTimeout time.Duration) *QUICSession {
	if queueSize <= 0 {
		queueSize = SendQueueSize
	}
	if readTimeout <= 0 {
		readTimeout = ReadTimeout
	}
	if writeTimeout <= 0 {
		writeTimeout = WriteTimeout
	}

	return &QUICSession{
		conn:         conn,
		authDone:     make(chan struct{}),
		done:         make(chan struct{}),
		sendQueue:    make(chan []byte, queueSize),
		lastActive:   time.Now(),
		readTimeout:  readTimeout,
		writeTimeout: writeTimeout,
	}
}

func (s *QUICSession) Run() {
	// 1. 第一步：读取客户端鉴权包，拿到 uid
	uid, err := s.readAuth()
	if err != nil || uid == "" {
		s.Close()
		return
	}

	// 2. 鉴权成功：设置 uid、关闭 authDone 信号
	s.setUID(uid)

	// 3. 启动三大后台协程
	go s.readLoop()      // 循环接收客户端上行消息
	go s.writeLoop()     // 循环从队列取消息、下发给客户端
	go s.heartbeatLoop() // 心跳超时检测

	// 4. 阻塞等待：会话关闭信号，连接断开后整个 Run 退出
	<-s.Done()
}

func (s *QUICSession) WaitAuthDone() string {
	// 1. 阻塞等待：认证完成 或 会话关闭
	select {
	case <-s.authDone: // 分支1：收到「认证完成」信号，跳出阻塞
	case <-s.done: // 分支2：会话提前关闭/断开，也跳出阻塞
	}

	// 2. 加读锁，安全读取 uid
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.uid
}

func (s *QUICSession) Enqueue(msg []byte) bool {
	// 1. 空消息直接拒绝
	if len(msg) == 0 {
		return false
	}

	// 2. 三分支 select 非阻塞判断
	select {
	case <-s.done:
		// 会话已关闭（done 通道已关闭），不再接收新消息
		return false

	case s.sendQueue <- msg:
		// 队列未满，成功写入消息
		return true

	default:
		// 队列已满，不阻塞调用方，直接返回失败
		return false
	}
}

func (s *QUICSession) Close() {
	// closeOnce 保证内部匿名函数全局仅执行一次
	s.closeOnce.Do(func() {
		// 1. 关闭全局退出通道 done，广播关闭信号
		close(s.done)
		// 2. 关闭底层 QUIC 连接，附带错误码与描述
		_ = s.conn.CloseWithError(0, "session closed")
	})
}

func (s *QUICSession) Done() <-chan struct{} {
	return s.done
}

func (s *QUICSession) UID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.uid
}

func (s *QUICSession) SetTouchHandler(fn func(uid string)) {
	s.mu.Lock()
	s.onTouch = fn
	s.mu.Unlock()
}

// 鉴权登录
func (s *QUICSession) readAuth() (string, error) {
	// 1. 新建带超时的上下文，控制鉴权读取最长等待时间
	ctx, cancel := context.WithTimeout(context.Background(), s.readTimeout)
	defer cancel()
	// 2. 接受客户端第一条 QUIC 数据流
	stream, err := s.conn.AcceptStream(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = stream.Close() }()
	// 3. 读取流数据，限制最大 1024 字节
	payload, err := readStreamPayload(stream, 1024)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(payload)), nil
}

// 接受消息
func (s *QUICSession) readLoop() {
	for {
		//QUIC一条底层连接可以并发多条独立Stream, 所以循环阻塞等待新流
		stream, err := s.conn.AcceptStream(context.Background())
		if err != nil {
			s.Close()
			return
		}
		//每条新流单独子协程处理
		//串行收流， 并行处理
		go s.handleStream(stream)
	}
}

func (s *QUICSession) handleStream(stream *quic.Stream) {
	defer func() { _ = stream.Close() }()

	payload, err := readStreamPayload(stream, 64*1024)
	if err != nil {
		return
	}
	//更新最后活跃时间
	s.touch()
	if bytes.Equal(bytes.TrimSpace(payload), []byte("PING")) {
		_ = s.Enqueue([]byte("PONG"))
	}
}

func (s *QUICSession) writeLoop() {
	for {
		select {
		case <-s.done:
			return
		case msg := <-s.sendQueue:
			if err := s.writeMessage(msg); err != nil {
				s.Close()
				return
			}
		}
	}
}

func (s *QUICSession) writeMessage(msg []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.writeTimeout)
	defer cancel()
	//同步创建一条新QUIC Stream用于发消息， 每条消息占一个流
	stream, err := s.conn.OpenStreamSync(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = stream.Close() }()

	_, err = stream.Write(msg)
	return err
}

func (s *QUICSession) heartbeatLoop() {
	ticker := time.NewTicker(HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C: //定时器响
			s.mu.RLock()
			lastActive := s.lastActive
			s.mu.RUnlock()

			if time.Since(lastActive) > s.readTimeout {
				s.Close()
				return
			}
		}
	}
}

func (s *QUICSession) setUID(uid string) {
	s.mu.Lock()
	s.uid = uid
	s.lastActive = time.Now()
	s.mu.Unlock()

	s.authOnce.Do(func() {
		close(s.authDone)
	})
}

func (s *QUICSession) touch() {
	s.mu.Lock()
	s.lastActive = time.Now() // 刷新活跃时间，续期本地超时计时
	uid := s.uid              // 快照：拷贝当前 uid
	onTouch := s.onTouch      // 快照：拷贝回调函数
	s.mu.Unlock()
	/*
		客户端上行数据
		      ↓
		touch() 刷新本地 lastActive
		      ↓
		执行 onTouch 回调
		      ↓
		网关 → PresenceStore → Redis 续期在线状态
	*/
	if onTouch != nil && uid != "" {
		onTouch(uid)
	}
}

func readStreamPayload(stream *quic.Stream, limit int64) ([]byte, error) {
	/*
			，最多只读取 N 个字节。
		允许多读 1 字节，用来区分「正常包」和「超大包」。
	*/
	reader := io.LimitReader(stream, limit+1)
	payload, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if int64(len(payload)) > limit {
		return nil, errors.New("stream payload too large")
	}
	return payload, nil
}
