package container

import (
	"context"
	"go-admin/api"
	"go-admin/internal/repository"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

// Container 依赖注入容器
type Container struct {
	UserService *api.UserService
	PostAPI     *api.PostAPI
	CommentAPI  *api.CommentAPI
	InteractAPI *api.InteractAPI
	SocialAPI   *api.SocialAPI
	MessageAPI  *api.MessageAPI
	WSAPI       *api.WSAPI
	// 其他服务可以继续添加...
}

// NewContainer 创建依赖注入容器
func NewContainer(db *gorm.DB, redisClient *redis.Client, jwtSecret []byte) *Container {
	ctx := context.Background()

	// 创建Repository实现
	userDB := &repository.GormUserDB{DB: db}
	userCache := &repository.RedisUserCache{Client: redisClient}
	postRepo := repository.NewGormPostRepository(db)
	commentRepo := repository.NewGormCommentRepository(db)
	interactRepo := repository.NewGormInteractRepository(db)
	socialRepo := repository.NewGormSocialRepository(db)
	messageRepo := repository.NewGormMessageRepository(db)

	// 创建JWT配置
	jwtCfg := &repository.DefaultJWTConfig{Secret: jwtSecret}

	// 创建服务实例
	userService := api.NewUserService(userDB, userCache, jwtCfg, jwtSecret, ctx)
	postAPI := api.NewPostAPI(postRepo)
	commentAPI := api.NewCommentAPI(commentRepo)
	interactAPI := api.NewInteractAPI(interactRepo)
	socialAPI := api.NewSocialAPI(socialRepo)
	messageAPI := api.NewMessageAPI(messageRepo)
	wsAPI := api.NewWSAPI(messageRepo, jwtSecret)

	return &Container{
		UserService: userService,
		PostAPI:     postAPI,
		CommentAPI:  commentAPI,
		InteractAPI: interactAPI,
		SocialAPI:   socialAPI,
		MessageAPI:  messageAPI,
		WSAPI:       wsAPI,
	}
}
