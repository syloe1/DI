package gateway

import (
	"context"
	"sync"

	"github.com/quic-go/quic-go"
)

type QUICServer struct {
	listener      *quic.Listener // QUIC 监听器，负责接收客户端连接
	localSessions sync.Map       // 本地会话表：key=用户UID，value=*QUICSession（并发安全字典）
	presence      PresenceStore  // 在线状态存储（对接 Redis 等，同步全局上下线）
	quit          chan struct{}  // 服务关闭信号通道
	closeOnce     sync.Once      // 保证 Close 方法只执行一次（防重复关闭）
}

// 编译期校验：确保 QUICServer 完整实现 SessionManager 接口

var _ SessionManager = (*QUICServer)(nil)

func NewQUICServer(listener *quic.Listener) *QUICServer {
	return &QUICServer{
		listener: listener,
		quit:     make(chan struct{}),
	}
}

func NewQUICServerWithPresence(listener *quic.Listener, presence PresenceStore) *QUICServer {
	return &QUICServer{
		listener: listener,
		presence: presence,
		quit:     make(chan struct{}),
	}
}
func (s *QUICServer) Run(ctx context.Context) error {
	for {
		// 监听全局取消 / 服务关闭信号
		select {
		case <-ctx.Done():
			s.Close()
			return ctx.Err()
		case <-s.quit:
			return nil
		default:
		}

		// 阻塞等待客户端 QUIC 连接
		conn, err := s.listener.Accept(ctx)
		if err != nil {
			// 连接出错，判断是否是主动关闭
			select {
			case <-ctx.Done():
				s.Close()
				return ctx.Err()
			case <-s.quit:
				return nil
			default:
				return err
			}
		}

		// 新建会话对象
		session := NewQUICSession(conn, SendQueueSize, ReadTimeout, WriteTimeout)
		go session.Run()                        // 启动会话读写协程
		go s.registerWhenAuthenticated(session) // 异步等待认证、再注册会话
	}
}
func (s *QUICServer) Register(uid string, session *QUICSession) {
	if uid == "" || session == nil {
		return
	}

	// 互踢逻辑：同一用户已存在会话，先关闭旧连接
	if old, ok := s.localSessions.Load(uid); ok {
		if oldSession, ok := old.(*QUICSession); ok && oldSession != session {
			oldSession.Close()
		}
	}

	// 写入本地会话表
	s.localSessions.Store(uid, session)

	// 注册心跳回调：客户端有数据交互时，刷新全局在线状态
	session.SetTouchHandler(func(uid string) {
		if s.presence != nil {
			_ = s.presence.RefreshOnline(context.Background(), uid)
		}
	})

	// 同步到全局在线存储（Redis）：标记用户上线
	if s.presence != nil {
		_ = s.presence.RegisterOnline(context.Background(), uid)
	}
}
func (s *QUICServer) Unregister(uid string, session *QUICSession) {
	if uid == "" || session == nil {
		return
	}

	// 防止误删：当前会话和存储的不一致，直接返回
	//只有当前运行的会话才能删除自己。
	current, ok := s.localSessions.Load(uid)
	if !ok || current != session {
		return
	}

	// 删除本地会话
	s.localSessions.Delete(uid)
	// 同步全局存储：标记用户下线
	if s.presence != nil {
		_ = s.presence.UnregisterOnline(context.Background(), uid)
	}
}

// 消息推送
func (s *QUICServer) Enqueue(uid string, msg []byte) bool {
	if uid == "" || len(msg) == 0 {
		return false
	}

	// 查找本地会话
	value, ok := s.localSessions.Load(uid)
	if !ok {
		return false
	}

	session, ok := value.(*QUICSession)
	if !ok || session == nil {
		return false
	}
	// 往会话的发送队列塞消息
	return session.Enqueue(msg)
}

func (s *QUICServer) IsOnline(uid string) bool {
	if uid == "" {
		return false
	}

	_, ok := s.localSessions.Load(uid)
	return ok
}

// 获取当前网关全部在线用户ID
func (s *QUICServer) LocalUserIDs() []string {
	users := make([]string, 0)
	s.localSessions.Range(func(key, value any) bool {
		if uid, ok := key.(string); ok {
			users = append(users, uid)
		}
		return true
	})
	return users
}

func (s *QUICServer) Close() {
	s.closeOnce.Do(func() { // 确保只执行一次
		close(s.quit)          // 发送服务退出信号
		_ = s.listener.Close() // 关闭 QUIC 监听

		// 逐个关闭所有本地会话、清空会话表
		s.localSessions.Range(func(key, value any) bool {
			if session, ok := value.(*QUICSession); ok {
				session.Close()
			}
			s.localSessions.Delete(key)
			return true
		})

		// 关闭全局在线状态组件
		if s.presence != nil {
			_ = s.presence.Shutdown(context.Background())
		}
	})
}
func (s *QUICServer) registerWhenAuthenticated(session *QUICSession) {
	// 阻塞等待客户端完成身份认证，拿到 UID
	uid := session.WaitAuthDone()
	if uid == "" {
		// 认证失败/无UID，直接关闭连接
		session.Close()
		return
	}

	// 认证成功 → 注册上线
	s.Register(uid, session)
	// 阻塞直到会话断开
	<-session.Done()
	// 会话结束 → 注销下线
	s.Unregister(uid, session)
}
