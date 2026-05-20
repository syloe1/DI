package gateway

type SessionManager interface {
	Register(uid string, session *QUICSession)
	Unregister(uid string, session *QUICSession)
	Enqueue(uid string, msg []byte) bool
	IsOnline(uid string) bool
	LocalUserIDs() []string
}
