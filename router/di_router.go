package router

import (
	"go-admin/internal/container"
	"go-admin/middleware"

	"github.com/gin-gonic/gin"
)

// InitDependencyInjectionRouter 初始化依赖注入版本的路由
func InitDependencyInjectionRouter(container *container.Container) *gin.Engine {
	r := gin.Default()
	r.Use(middleware.Cors())

	// ======================
	// 【公开接口】不需要登录
	// ======================
	r.POST("/user/register", container.UserService.Register)
	r.POST("/user/login", container.UserService.Login)
	r.GET("/post/list", container.PostAPI.GetPostList)
	r.GET("/post/:id", container.PostAPI.GetPost)
	r.GET("/post/user/:id", container.PostAPI.GetUserPosts)
	r.GET("/post/user/:id/liked", container.PostAPI.GetUserLikedPosts)
	r.GET("/post/user/:id/collected", container.PostAPI.GetUserCollectedPosts)
	r.GET("/comment/post/:post_id", container.CommentAPI.GetPostComments)
	r.GET("/interact/count/:post_id", container.InteractAPI.GetInteractCount)
	r.GET("/user/search", container.UserService.SearchUser)
	r.GET("/user/:id", container.UserService.GetUser)

	// WebSocket - 独立路径，通过query参数token验证
	r.GET("/ws", container.WSAPI.HandleWebSocket)

	// ======================
	// 【需要登录的接口】
	// =======================
	auth := r.Group("/auth")
	auth.Use(middleware.JWTAuth(container.JWTSecret))
	{
		// 用户相关
		auth.GET("/user/list", container.UserService.GetUserList)
		auth.GET("/user/:id", container.UserService.GetUser)
		auth.POST("/user/batch-roles", container.UserService.BatchGetUserRoles)
		auth.PUT("/user/:id", container.UserService.UpdateUser)
		auth.PUT("/user/password/:id", container.UserService.ChangePassword)

		// 帖子相关
		auth.POST("/post/create", container.PostAPI.CreatePost)
		auth.GET("/post/my", container.PostAPI.GetMyPostList)
		auth.PUT("/post/:id", container.PostAPI.UpdatePost)
		auth.DELETE("/post/:id", container.PostAPI.DeletePost)

		// 评论相关
		auth.POST("/comment/create", container.CommentAPI.CreateComment)
		auth.DELETE("/comment/:id", container.CommentAPI.DeleteComment)
		auth.PUT("/comment/:id", container.CommentAPI.UpdateComment)
		auth.GET("/comment/my", container.CommentAPI.GetUserComments)

		// 互动相关
		auth.POST("/interact/like/:post_id", container.InteractAPI.ToggleLike)
		auth.POST("/interact/dislike/:post_id", container.InteractAPI.ToggleDislike)
		auth.POST("/interact/collect/:post_id", container.InteractAPI.ToggleCollect)
		auth.POST("/interact/share/:post_id", container.InteractAPI.Share)
		auth.GET("/interact/status/:post_id", container.InteractAPI.GetInteractStatus)

		// 社交相关
		auth.POST("/social/follow/:uid", container.SocialAPI.FollowUser)
		auth.POST("/social/block/:uid", container.SocialAPI.BlockUser)
		auth.GET("/social/relation/:uid", container.SocialAPI.GetRelationStatus)
		auth.GET("/social/follows", container.SocialAPI.GetFollowList)
		auth.GET("/social/followers", container.SocialAPI.GetFollowerList)
		auth.GET("/social/blocks", container.SocialAPI.GetBlockList)

		// 消息相关
		auth.GET("/message/conversations", container.MessageAPI.GetConversations)
		auth.GET("/message/list", container.MessageAPI.GetMessageList)
		auth.POST("/message/send", container.MessageAPI.SendMessage)

		// 管理员功能
		admin := auth.Group("/")
		admin.Use(middleware.AdminAuth())
		{
			admin.POST("/user/add", container.UserService.AddUser)
			admin.DELETE("/user/:id", container.UserService.DeleteUser)
		}
	}

	return r
}