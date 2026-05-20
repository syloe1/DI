package gateway

import (
	"context"
	"sync"

	"github.com/quic-go/quic-go"
)

type QUICServer struct {
	listener      *quic.Listener
	localSessions sync.Map
	presence      PresenceStore
	quit          chan struct{}
	closeOnce     sync.Once
}

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
		select {
		case <-ctx.Done():
			s.Close()
			return ctx.Err()
		case <-s.quit:
			return nil
		default:
		}

		conn, err := s.listener.Accept(ctx)
		if err != nil {
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

		session := NewQUICSession(conn, SendQueueSize, ReadTimeout, WriteTimeout)
		go session.Run()
		go s.registerWhenAuthenticated(session)
	}
}

func (s *QUICServer) Register(uid string, session *QUICSession) {
	if uid == "" || session == nil {
		return
	}

	if old, ok := s.localSessions.Load(uid); ok {
		if oldSession, ok := old.(*QUICSession); ok && oldSession != session {
			oldSession.Close()
		}
	}

	s.localSessions.Store(uid, session)
	session.SetTouchHandler(func(uid string) {
		if s.presence != nil {
			_ = s.presence.RefreshOnline(context.Background(), uid)
		}
	})
	if s.presence != nil {
		_ = s.presence.RegisterOnline(context.Background(), uid)
	}
}

func (s *QUICServer) Unregister(uid string, session *QUICSession) {
	if uid == "" || session == nil {
		return
	}

	current, ok := s.localSessions.Load(uid)
	if !ok || current != session {
		return
	}

	s.localSessions.Delete(uid)
	if s.presence != nil {
		_ = s.presence.UnregisterOnline(context.Background(), uid)
	}
}

func (s *QUICServer) Enqueue(uid string, msg []byte) bool {
	if uid == "" || len(msg) == 0 {
		return false
	}

	value, ok := s.localSessions.Load(uid)
	if !ok {
		return false
	}

	session, ok := value.(*QUICSession)
	if !ok || session == nil {
		return false
	}

	return session.Enqueue(msg)
}

func (s *QUICServer) IsOnline(uid string) bool {
	if uid == "" {
		return false
	}

	_, ok := s.localSessions.Load(uid)
	return ok
}

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
	s.closeOnce.Do(func() {
		close(s.quit)
		_ = s.listener.Close()
		s.localSessions.Range(func(key, value any) bool {
			if session, ok := value.(*QUICSession); ok {
				session.Close()
			}
			s.localSessions.Delete(key)
			return true
		})
		if s.presence != nil {
			_ = s.presence.Shutdown(context.Background())
		}
	})
}

func (s *QUICServer) registerWhenAuthenticated(session *QUICSession) {
	uid := session.WaitAuthDone()
	if uid == "" {
		session.Close()
		return
	}

	s.Register(uid, session)
	<-session.Done()
	s.Unregister(uid, session)
}
