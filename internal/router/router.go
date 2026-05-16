package router

import (
	"go-admin/internal/container"
	"go-admin/internal/middleware"

	"github.com/gin-gonic/gin"
)

// InitDependencyInjectionRouter wires middleware and handlers.
func InitDependencyInjectionRouter(container *container.Container) *gin.Engine {
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.RequestLogger(container.Logger))
	r.Use(middleware.CustomRecovery(container.Logger))
	r.Use(middleware.Cors())

	// Public endpoints.
	r.POST("/user/register", middleware.RateLimit("register", 1.0/30.0, 2, middleware.ClientIPKey), container.UserHandler.Register)
	r.POST("/user/login", middleware.RateLimit("login", 1, 5, middleware.ClientIPKey), container.UserHandler.Login)
	r.GET("/post/list", container.PostHandler.GetPostList)
	r.GET("/post/hot", container.PostHandler.GetHotPosts)
	r.GET("/post/:id", container.PostHandler.GetPost)
	r.GET("/post/user/:id", container.PostHandler.GetUserPosts)
	r.GET("/post/user/:id/liked", container.PostHandler.GetUserLikedPosts)
	r.GET("/post/user/:id/collected", container.PostHandler.GetUserCollectedPosts)
	r.GET("/comment/post/:post_id", container.CommentHandler.GetPostComments)
	r.GET("/interact/count/:post_id", container.InteractHandler.GetInteractCount)
	r.GET("/user/search", container.UserHandler.SearchUser)
	r.GET("/user/online/list", container.UserHandler.GetOnlineUsers)
	r.GET("/user/:id/online", container.UserHandler.GetUserOnlineStatus)
	r.GET("/user/:id", container.UserHandler.GetUser)

	// WebSocket entrypoint authenticated by query token.
	r.GET("/ws", container.WSHandler.HandleWebSocket)

	// Authenticated endpoints.
	auth := r.Group("/auth")
	auth.Use(middleware.JWTAuth(container.JWTSecret))
	{
		// User endpoints.
		auth.GET("/user/list", container.UserHandler.GetUserList)
		auth.GET("/user/:id", container.UserHandler.GetUser)
		auth.POST("/user/logout", container.UserHandler.Logout)
		auth.POST("/user/batch-roles", container.UserHandler.BatchGetUserRoles)
		auth.PUT("/user/:id", container.UserHandler.UpdateUser)
		auth.PUT("/user/password/:id", container.UserHandler.ChangePassword)

		// Post endpoints.
		auth.POST("/post/create", middleware.RateLimit("post:create", 0.1, 3, middleware.UserIDKey), container.PostHandler.CreatePost)
		auth.GET("/post/my", container.PostHandler.GetMyPostList)
		auth.PUT("/post/:id", container.PostHandler.UpdatePost)
		auth.DELETE("/post/:id", container.PostHandler.DeletePost)

		// Comment endpoints.
		auth.POST("/comment/create", container.CommentHandler.CreateComment)
		auth.DELETE("/comment/:id", container.CommentHandler.DeleteComment)
		auth.PUT("/comment/:id", container.CommentHandler.UpdateComment)
		auth.GET("/comment/my", container.CommentHandler.GetUserComments)

		// Interaction endpoints.
		auth.POST("/interact/like/:post_id", middleware.RateLimit("interact:like", 5, 10, middleware.UserIDKey), container.InteractHandler.ToggleLike)
		auth.POST("/interact/dislike/:post_id", container.InteractHandler.ToggleDislike)
		auth.POST("/interact/collect/:post_id", container.InteractHandler.ToggleCollect)
		auth.POST("/interact/share/:post_id", container.InteractHandler.Share)
		auth.GET("/interact/status/:post_id", container.InteractHandler.GetInteractStatus)

		// Social endpoints.
		auth.POST("/social/follow/:uid", container.SocialHandler.FollowUser)
		auth.POST("/social/block/:uid", container.SocialHandler.BlockUser)
		auth.GET("/social/relation/:uid", container.SocialHandler.GetRelationStatus)
		auth.GET("/social/follows", container.SocialHandler.GetFollowList)
		auth.GET("/social/followers", container.SocialHandler.GetFollowerList)
		auth.GET("/social/blocks", container.SocialHandler.GetBlockList)

		// Message endpoints.
		auth.GET("/message/conversations", container.MessageHandler.GetConversations)
		auth.GET("/message/list", container.MessageHandler.GetMessageList)
		auth.POST("/message/send", middleware.RateLimit("message:send", 2, 6, middleware.UserIDKey), container.MessageHandler.SendMessage)
		auth.DELETE("/message/:id", container.MessageHandler.DeleteMessage)

		// Group endpoints.
		auth.POST("/groups", container.GroupHandler.CreateGroup)
		auth.GET("/groups/my", container.GroupHandler.GetMyGroups)
		auth.GET("/groups/:id", container.GroupHandler.GetGroupDetail)
		auth.GET("/groups/:id/members", container.GroupHandler.GetGroupMembers)
		auth.GET("/groups/:id/join-requests", container.GroupHandler.GetJoinRequests)
		auth.POST("/groups/:id/admins", container.GroupHandler.SetAdmin)
		auth.DELETE("/groups/:id/admins/:user_id", container.GroupHandler.CancelAdmin)
		auth.DELETE("/groups/:id/members/:user_id", container.GroupHandler.KickMember)
		auth.POST("/groups/:id/transfer-owner", container.GroupHandler.TransferOwner)
		auth.POST("/groups/:id/dissolve", container.GroupHandler.DissolveGroup)
		auth.POST("/groups/:id/invitations", container.GroupHandler.InviteMember)
		auth.POST("/groups/invitations/:id/review", container.GroupHandler.ReviewInvitation)
		auth.POST("/groups/:id/join-requests", container.GroupHandler.ApplyJoinGroup)
		auth.POST("/groups/join-requests/:id/review", container.GroupHandler.ReviewJoinRequest)
		auth.POST("/groups/:id/leave", container.GroupHandler.LeaveGroup)
		auth.POST("/groups/:id/messages", container.GroupHandler.SendGroupMessage)
		auth.GET("/groups/:id/messages", container.GroupHandler.GetGroupMessages)

		// Admin-only endpoints.
		admin := auth.Group("/")
		admin.Use(middleware.AdminAuth())
		{
			admin.POST("/user/add", container.UserHandler.AddUser)
			admin.DELETE("/user/:id", container.UserHandler.DeleteUser)
		}
	}

	return r
}
