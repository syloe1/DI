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

	return &Container{
		Config:      cfg,
		Logger:      appLogger,
		DB:          db,
		Redis:       redisClient,
		JWTSecret:   jwtSecret,
		UserService: api.NewUserService(userDB, userCache, jwtCfg, jwtSecret, ctx),
		PostAPI:     api.NewPostAPI(postRepo),
		CommentAPI:  api.NewCommentAPI(commentRepo),
		InteractAPI: api.NewInteractAPI(interactRepo),
		SocialAPI:   api.NewSocialAPI(socialRepo),
		MessageAPI:  api.NewMessageAPI(messageRepo),
		WSAPI:       api.NewWSAPI(messageRepo, jwtSecret),
	}
}
