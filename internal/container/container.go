package container

import (
	"context"
	"log"

	"go-admin/api"
	"go-admin/config"
	"go-admin/internal/repository"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

type Container struct {
	Config    *config.App
	Logger    *log.Logger
	DB        *gorm.DB
	Redis     *redis.Client
	JWTSecret []byte

	UserService *api.UserService
	PostAPI     *api.PostAPI
	CommentAPI  *api.CommentAPI
	InteractAPI *api.InteractAPI
	SocialAPI   *api.SocialAPI
	MessageAPI  *api.MessageAPI
	WSAPI       *api.WSAPI
}

func NewContainer(cfg *config.App, db *gorm.DB, redisClient *redis.Client, appLogger *log.Logger) *Container {
	ctx := context.Background()
	jwtSecret := []byte(cfg.GetJwtConfig().Secret)

	userDB := &repository.GormUserDB{DB: db}
	userCache := &repository.RedisUserCache{Client: redisClient}
	postRepo := repository.NewGormPostRepository(db)
	commentRepo := repository.NewGormCommentRepository(db)
	interactRepo := repository.NewGormInteractRepository(db)
	socialRepo := repository.NewGormSocialRepository(db)
	messageRepo := repository.NewGormMessageRepository(db)
	jwtCfg := &repository.DefaultJWTConfig{Secret: jwtSecret}

	userService := api.NewUserService(userDB, userCache, jwtCfg, jwtSecret, ctx)
	postAPI := api.NewPostAPI(postRepo)
	commentAPI := api.NewCommentAPI(commentRepo)
	interactAPI := api.NewInteractAPI(interactRepo)
	socialAPI := api.NewSocialAPI(socialRepo)
	messageAPI := api.NewMessageAPI(messageRepo)
	wsAPI := api.NewWSAPI(messageRepo, jwtSecret)

	postAPI.SetInteractExtension(interactAPI)
	userService.SetInteractExtension(interactAPI)
	userService.SetSocialExtension(socialAPI)
	userService.SetMessageExtension(messageAPI)
	userService.SetWSExtension(wsAPI)

	return &Container{
		Config:      cfg,
		Logger:      appLogger,
		DB:          db,
		Redis:       redisClient,
		JWTSecret:   jwtSecret,
		UserService: userService,
		PostAPI:     postAPI,
		CommentAPI:  commentAPI,
		InteractAPI: interactAPI,
		SocialAPI:   socialAPI,
		MessageAPI:  messageAPI,
		WSAPI:       wsAPI,
	}
}
