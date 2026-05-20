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
	ReadTimeout       = 90 * time.Second
	WriteTimeout      = 30 * time.Second
	HeartbeatInterval = 30 * time.Second
	SendQueueSize     = 1024
)

type QUICSession struct {
	conn         *quic.Conn
	uid          string
	authDone     chan struct{}
	done         chan struct{}
	sendQueue    chan []byte
	lastActive   time.Time
	readTimeout  time.Duration
	writeTimeout time.Duration
	closeOnce    sync.Once
	authOnce     sync.Once
	mu           sync.RWMutex
	onTouch      func(uid string)
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
	uid, err := s.readAuth()
	if err != nil || uid == "" {
		s.Close()
		return
	}

	s.setUID(uid)
	go s.readLoop()
	go s.writeLoop()
	go s.heartbeatLoop()

	<-s.Done()
}

func (s *QUICSession) WaitAuthDone() string {
	select {
	case <-s.authDone:
	case <-s.done:
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.uid
}

func (s *QUICSession) Enqueue(msg []byte) bool {
	if len(msg) == 0 {
		return false
	}

	select {
	case <-s.done:
		return false
	case s.sendQueue <- msg:
		return true
	default:
		return false
	}
}

func (s *QUICSession) Close() {
	s.closeOnce.Do(func() {
		close(s.done)
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

func (s *QUICSession) readAuth() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.readTimeout)
	defer cancel()

	stream, err := s.conn.AcceptStream(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = stream.Close() }()

	payload, err := readStreamPayload(stream, 1024)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(payload)), nil
}

func (s *QUICSession) readLoop() {
	for {
		stream, err := s.conn.AcceptStream(context.Background())
		if err != nil {
			s.Close()
			return
		}

		go s.handleStream(stream)
	}
}

func (s *QUICSession) handleStream(stream *quic.Stream) {
	defer func() { _ = stream.Close() }()

	payload, err := readStreamPayload(stream, 64*1024)
	if err != nil {
		return
	}

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
		case <-ticker.C:
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
	s.lastActive = time.Now()
	uid := s.uid
	onTouch := s.onTouch
	s.mu.Unlock()
	if onTouch != nil && uid != "" {
		onTouch(uid)
	}
}

func readStreamPayload(stream *quic.Stream, limit int64) ([]byte, error) {
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
