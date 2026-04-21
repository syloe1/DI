package container

import (
	"context"
	"log"

	"go-admin/config"
	"go-admin/internal/dao"
	"go-admin/internal/handler"
	"go-admin/internal/service"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

type Container struct {
	Config    *config.App
	Logger    *log.Logger
	DB        *gorm.DB
	Redis     *redis.Client
	JWTSecret []byte

	UserService     *service.UserService
	UserHandler     *handler.UserHandler
	PostService     *service.PostService
	PostHandler     *handler.PostHandler
	WSService       *service.WSService
	WSHandler       *handler.WSHandler
	CommentService  *service.CommentService
	CommentHandler  *handler.CommentHandler
	MessageService  *service.MessageService
	MessageHandler  *handler.MessageHandler
	SocialService   *service.SocialService
	SocialHandler   *handler.SocialHandler
	InteractService *service.InteractService
	InteractHandler *handler.InteractHandler
}

func NewContainer(cfg *config.App, db *gorm.DB, redisClient *redis.Client, appLogger *log.Logger) *Container {
	ctx := context.Background()
	jwtSecret := []byte(cfg.GetJwtConfig().Secret)

	userDB := &dao.GormUserDB{DB: db}
	userCache := &dao.RedisUserCache{Client: redisClient}
	postRepo := dao.NewGormPostRepository(db)
	commentRepo := dao.NewGormCommentRepository(db)
	interactRepo := dao.NewGormInteractRepository(db)
	socialRepo := dao.NewGormSocialRepository(db)
	messageRepo := dao.NewGormMessageRepository(db)
	jwtCfg := &dao.DefaultJWTConfig{Secret: jwtSecret}

	userService := service.NewUserService(userDB, userCache, jwtCfg, jwtSecret, ctx)
	userHandler := handler.NewUserHandler(userService)
	postService := service.NewPostService(postRepo, userCache, ctx)
	postHandler := handler.NewPostHandler(postService)
	wsService := service.NewWSService(messageRepo, userCache, ctx, jwtSecret)
	wsHandler := handler.NewWSHandler(wsService)
	commentService := service.NewCommentService(commentRepo)
	commentHandler := handler.NewCommentHandler(commentService)
	messageService := service.NewMessageService(messageRepo, userCache, ctx)
	messageHandler := handler.NewMessageHandler(messageService)
	socialService := service.NewSocialService(socialRepo)
	socialHandler := handler.NewSocialHandler(socialService)
	interactService := service.NewInteractService(interactRepo, userCache, ctx)
	interactHandler := handler.NewInteractHandler(interactService)

	return &Container{
		Config:          cfg,
		Logger:          appLogger,
		DB:              db,
		Redis:           redisClient,
		JWTSecret:       jwtSecret,
		UserService:     userService,
		UserHandler:     userHandler,
		PostService:     postService,
		PostHandler:     postHandler,
		WSService:       wsService,
		WSHandler:       wsHandler,
		CommentService:  commentService,
		CommentHandler:  commentHandler,
		MessageService:  messageService,
		MessageHandler:  messageHandler,
		SocialService:   socialService,
		SocialHandler:   socialHandler,
		InteractService: interactService,
		InteractHandler: interactHandler,
	}
}
