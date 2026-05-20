package gateway

import "context"

type PresenceStore interface {
	//用户上线
	RegisterOnline(ctx context.Context, uid string) error
	//用户下线
	UnregisterOnline(ctx context.Context, uid string) error
	//刷新在线状态
	RefreshOnline(ctx context.Context, uid string) error
	//服务关闭
	Shutdown(ctx context.Context) error
}
