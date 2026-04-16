package router

import (
	"go-admin/internal/container"
	"go-admin/middleware"

	"github.com/gin-gonic/gin"
)

func InitDependencyInjectionRouter(container *container.Container) *gin.Engine {
	r := gin.Default()
	r.Use(middleware.Cors())

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
	r.GET("/ws", container.WSAPI.HandleWebSocket)

	auth := r.Group("/auth")
	auth.Use(middleware.JWTAuth(container.JWTSecret))
	{
		auth.GET("/user/list", container.UserService.GetUserList)
		auth.GET("/user/:id", container.UserService.GetUser)
		auth.POST("/user/batch-roles", container.UserService.BatchGetUserRoles)
		auth.PUT("/user/:id", container.UserService.UpdateUser)
		auth.PUT("/user/password/:id", container.UserService.ChangePassword)

		auth.POST("/post/create", container.PostAPI.CreatePost)
		auth.GET("/post/my", container.PostAPI.GetMyPostList)
		auth.PUT("/post/:id", container.PostAPI.UpdatePost)
		auth.DELETE("/post/:id", container.PostAPI.DeletePost)

		auth.POST("/comment/create", container.CommentAPI.CreateComment)
		auth.DELETE("/comment/:id", container.CommentAPI.DeleteComment)
		auth.PUT("/comment/:id", container.CommentAPI.UpdateComment)
		auth.GET("/comment/my", container.CommentAPI.GetUserComments)

		auth.POST("/interact/like/:post_id", container.InteractAPI.ToggleLike)
		auth.POST("/interact/dislike/:post_id", container.InteractAPI.ToggleDislike)
		auth.POST("/interact/collect/:post_id", container.InteractAPI.ToggleCollect)
		auth.POST("/interact/share/:post_id", container.InteractAPI.Share)
		auth.GET("/interact/status/:post_id", container.InteractAPI.GetInteractStatus)

		auth.POST("/social/follow/:uid", container.SocialAPI.FollowUser)
		auth.POST("/social/block/:uid", container.SocialAPI.BlockUser)
		auth.GET("/social/relation/:uid", container.SocialAPI.GetRelationStatus)
		auth.GET("/social/follows", container.SocialAPI.GetFollowList)
		auth.GET("/social/followers", container.SocialAPI.GetFollowerList)
		auth.GET("/social/blocks", container.SocialAPI.GetBlockList)

		auth.GET("/message/conversations", container.MessageAPI.GetConversations)
		auth.GET("/message/list", container.MessageAPI.GetMessageList)
		auth.POST("/message/send", container.MessageAPI.SendMessage)

		admin := auth.Group("/")
		admin.Use(middleware.AdminAuth())
		{
			admin.POST("/user/add", container.UserService.AddUser)
			admin.DELETE("/user/:id", container.UserService.DeleteUser)
		}
	}

	return r
}
