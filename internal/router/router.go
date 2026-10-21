package router

//router-api
import (
	"net/http"

	"go-admin/internal/container"
	"go-admin/internal/middleware"
	"go-admin/pkg/core"

	"github.com/gin-gonic/gin"
)

// InitDependencyInjectionRouter wires middleware and handlers.
func InitDependencyInjectionRouter(container *container.Container) *gin.Engine {
	r := gin.New()
	// 全局中间件：所有请求都会经过
	r.Use(middleware.RequestID())
	r.Use(middleware.RequestLogger(container.Logger))
	r.Use(middleware.CustomRecovery(container.Logger))
	r.Use(middleware.Cors(container.Config.GetCORSOrigins()))

	// ========== 1. 公开路由组：无需登录 ==========
	public := r.Group("/")
	{
		// 健康检查（Docker healthcheck / 负载均衡探活）
		public.GET("/healthz", func(c *gin.Context) { core.Success(c, gin.H{"status": "ok"}) })
		public.GET("/readyz", func(c *gin.Context) {
			if err := container.DB.Exec("SELECT 1").Error; err != nil {
				core.Fail(c, http.StatusServiceUnavailable, "mysql not ready")
				return
			}
			if err := container.Redis.Ping(c.Request.Context()).Err(); err != nil {
				core.Fail(c, http.StatusServiceUnavailable, "redis not ready")
				return
			}
			if container.Rabbit == nil || container.Rabbit.PublishChan() == nil || container.Rabbit.PublishChan().IsClosed() {
				core.Fail(c, http.StatusServiceUnavailable, "rabbitmq not ready")
				return
			}
			core.Success(c, gin.H{"status": "ok"})
		})

		// 用户登录注册
		// 注册限流收紧为「每 IP 20 秒 1 次」：连续批量注册第二次立即 429。
		// 演示/公网环境再叠加邀请码（APP_AUTH_REGISTER_INVITE_CODE）。
		public.POST("/user/register", middleware.RateLimit("register", 1.0/20.0, 1, middleware.ClientIPKey), container.UserHandler.Register)
		public.POST("/user/login", middleware.RateLimit("login", 1, 5, middleware.ClientIPKey), container.UserHandler.Login)
		// 注册页配置（是否要邀请码 / 是否开放体验 Demo），无需登录
		public.GET("/user/register/config", container.UserHandler.RegisterConfig)
		// 体验 Demo：免密登录只读演示账号（demo.enabled=true 且已 seed 时可用）
		public.POST("/user/demo-login", middleware.RateLimit("demo-login", 0.5, 3, middleware.ClientIPKey), container.UserHandler.DemoLogin)

		// 注意：用户搜索 / 在线状态属于内部数据，已移到 /auth 下（P0 未鉴权修复）。
		// 公开路由只保留健康检查、注册登录、Demo 入口与 WebSocket。

		// WebSocket 长连接入口
		public.GET("/ws", container.WSHandler.HandleWebSocket)
	}

	// ========== 2. 需要JWT登录鉴权的路由组 /auth ==========
	auth := r.Group("/auth")
	auth.Use(middleware.JWTAuth(container.JWTSecret, container.TokenRevocation))
	{
		// ---- 用户模块 /auth/user ----
		userGroup := auth.Group("/user")
		{
			userGroup.GET("/list", container.UserHandler.GetUserList)
			// 搜索 / 在线列表需要登录，避免未登录枚举用户与在线状态
			userGroup.GET("/search", container.UserHandler.SearchUser)
			userGroup.GET("/online/list", container.UserHandler.GetOnlineUsers)
			userGroup.GET("/:id/online", container.UserHandler.GetUserOnlineStatus)
			userGroup.GET("/:id", container.UserHandler.GetUser)
			userGroup.POST("/logout", container.UserHandler.Logout)
			userGroup.GET("/batch-roles", container.UserHandler.BatchGetUserRoles)
			userGroup.PUT("/batch-roles", container.UserHandler.BatchPutUserRoles)
			userGroup.PUT("/:id", container.UserHandler.UpdateUser)
			userGroup.PUT("/password/:id", container.UserHandler.ChangePassword)
		}

		// ---- 私聊消息模块 /auth/message ----
		msgGroup := auth.Group("/message")
		{
			msgGroup.GET("/conversations", container.MessageHandler.GetConversations)
			msgGroup.GET("/list", container.MessageHandler.GetMessageList)
			msgGroup.GET("/sync", container.MessageHandler.SyncMessages)
			msgGroup.POST("/send", middleware.RateLimit("message:send", 2, 6, middleware.UserIDKey), container.MessageHandler.SendMessage)
			msgGroup.DELETE("/:id", container.MessageHandler.DeleteMessage)
		}

		// ---- 群聊模块 /auth/groups ----
		groupGroup := auth.Group("/groups")
		{
			// 同时注册 /auth/groups 与 /auth/groups/，避免 gin 307 尾斜杠重定向
			// （反向代理/HTTPS 下重定向容易暴露内部地址或降级到 http）。
			groupGroup.POST("", container.GroupHandler.CreateGroup)
			groupGroup.POST("/", container.GroupHandler.CreateGroup)
			groupGroup.GET("/my", container.GroupHandler.GetMyGroups)
			groupGroup.GET("/:id", container.GroupHandler.GetGroupDetail)
			groupGroup.GET("/:id/members", container.GroupHandler.GetGroupMembers)
			groupGroup.GET("/:id/join-requests", container.GroupHandler.GetJoinRequests)
			groupGroup.POST("/:id/admins", container.GroupHandler.SetAdmin)
			groupGroup.DELETE("/:id/admins/:user_id", container.GroupHandler.CancelAdmin)
			groupGroup.DELETE("/:id/members/:user_id", container.GroupHandler.KickMember)
			groupGroup.POST("/:id/transfer-owner", container.GroupHandler.TransferOwner)
			groupGroup.POST("/:id/dissolve", container.GroupHandler.DissolveGroup)
			groupGroup.POST("/:id/invitations", container.GroupHandler.InviteMember)
			groupGroup.POST("/invitations/:id/review", container.GroupHandler.ReviewInvitation)
			groupGroup.POST("/:id/join-requests", container.GroupHandler.ApplyJoinGroup)
			groupGroup.POST("/join-requests/:id/review", container.GroupHandler.ReviewJoinRequest)
			groupGroup.POST("/:id/leave", container.GroupHandler.LeaveGroup)
			groupGroup.POST("/:id/messages", container.GroupHandler.SendGroupMessage)
			groupGroup.GET("/:id/messages", container.GroupHandler.GetGroupMessages)
		}

		// ---- 管理员接口（嵌套子路由，叠加管理员鉴权中间件） ----
		adminGroup := auth.Group("/admin")
		adminGroup.Use(middleware.AdminAuth())
		{
			adminGroup.POST("/user/add", container.UserHandler.AddUser)
			adminGroup.DELETE("/user/:id", container.UserHandler.DeleteUser)
		}
	}

	return r
}
