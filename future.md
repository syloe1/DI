# 新功能开发模板

这份文档配合当前项目结构使用：

```text
router -> handler -> service -> dao -> domain model
```
## 适用场景

当你要新增一个功能模块时，比如：

- `report`
- `notice`
- `tag`
- `favorite_list`
- `draft`

都可以按这份模板往下落。

## 推荐开发顺序

固定按这个顺序做，基本不会乱：

1. 先想清楚数据结构
2. 先建 `domain/model`
3. 再定义 `dto`
4. 再写 `dao`
5. 再写 `service`
6. 再写 `handler`
7. 最后接到 `router`

一句话总结：

```text
先定义数据，再写业务，最后接入接口
```

## 目录落点

假设新增一个模块，名字叫 `notice`，一般这样放：

```text
internal
├─ dao
│  └─ notice_dao.go
├─ domain
│  └─ model
│     └─ notice.go
├─ dto
│  └─ notice_dto.go
├─ handler
│  └─ notice_handler.go
├─ service
│  └─ notice_service.go
└─ router
   └─ router.go
```

如果功能比较大，也可以继续拆：

```text
notice_dao.go
notice_query.go
notice_cache.go
```

## 每一层写什么

### 1. `internal/domain/model/notice.go`

这里定义数据库模型。

适合放：

- Gorm struct
- 表字段
- 表关系

不适合放：

- Gin 参数绑定
- 接口响应结构
- 业务流程

示例：

```go
package model

import "time"

type Notice struct {
	ID        int64     `gorm:"primaryKey" json:"id"`
	UserID    int64     `gorm:"index;not null" json:"user_id"`
	Title     string    `gorm:"size:100;not null" json:"title"`
	Content   string    `gorm:"type:text;not null" json:"content"`
	IsRead    bool      `gorm:"default:false" json:"is_read"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
```

### 2. `internal/dto/notice_dto.go`

这里定义接口入参和出参。

原则：

- 前端传什么，DTO 就写什么
- 不要把数据库模型直接当请求结构来用

示例：

```go
package dto

type CreateNoticeRequest struct {
	Title   string `json:"title" binding:"required,max=100"`
	Content string `json:"content" binding:"required"`
}

type NoticeResponse struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	IsRead    bool   `json:"is_read"`
	CreatedAt string `json:"created_at"`
}
```

说明：

- `UserID` 不一定要让前端传，很多时候可以从登录态拿
- `CreatedAt` 在响应里可以转成前端更容易消费的格式

### 3. `internal/dao/notice_dao.go`

这里负责数据访问。

建议：

- 先定义接口，再给 Gorm 实现
- service 只依赖接口，不直接依赖具体实现

示例：

```go
package dao

import (
	"go-admin/internal/domain/model"

	"gorm.io/gorm"
)

type NoticeDAO interface {
	Create(notice *model.Notice) error
	GetByID(id int64) (*model.Notice, error)
	ListByUserID(userID int64, limit, offset int) ([]model.Notice, error)
	MarkRead(id int64, userID int64) error
}

type GormNoticeDAO struct {
	DB *gorm.DB
}

func NewGormNoticeDAO(db *gorm.DB) *GormNoticeDAO {
	return &GormNoticeDAO{DB: db}
}

func (d *GormNoticeDAO) Create(notice *model.Notice) error {
	return d.DB.Create(notice).Error
}

func (d *GormNoticeDAO) GetByID(id int64) (*model.Notice, error) {
	var notice model.Notice
	if err := d.DB.First(&notice, id).Error; err != nil {
		return nil, err
	}
	return &notice, nil
}

func (d *GormNoticeDAO) ListByUserID(userID int64, limit, offset int) ([]model.Notice, error) {
	var notices []model.Notice
	err := d.DB.
		Where("user_id = ?", userID).
		Order("id desc").
		Limit(limit).
		Offset(offset).
		Find(&notices).Error
	return notices, err
}

func (d *GormNoticeDAO) MarkRead(id int64, userID int64) error {
	return d.DB.Model(&model.Notice{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("is_read", true).Error
}
```

### 4. `internal/service/notice_service.go`

这里写业务逻辑。

适合放：

- 参数规则判断
- 权限判断
- 默认值填充
- 调用多个 dao
- 缓存更新

不适合放：

- Gin 上下文读写
- HTTP 响应返回

示例：

```go
package service

import (
	"errors"
	"go-admin/internal/dao"
	"go-admin/internal/domain/model"
)

var ErrNoticeNotFound = errors.New("notice not found")

type NoticeService struct {
	dao dao.NoticeDAO
}

func NewNoticeService(dao dao.NoticeDAO) *NoticeService {
	return &NoticeService{dao: dao}
}

func (s *NoticeService) CreateNotice(userID int64, title, content string) error {
	notice := model.Notice{
		UserID:  userID,
		Title:   title,
		Content: content,
		IsRead:  false,
	}
	return s.dao.Create(&notice)
}

func (s *NoticeService) ListUserNotices(userID int64, limit, offset int) ([]model.Notice, error) {
	return s.dao.ListByUserID(userID, limit, offset)
}

func (s *NoticeService) MarkNoticeRead(id int64, userID int64) error {
	return s.dao.MarkRead(id, userID)
}
```

### 5. `internal/handler/notice_handler.go`

这里负责接请求、绑参数、调 service、回响应。

不要把业务逻辑写得太厚。

示例：

```go
package handler

import (
	"go-admin/internal/dto"
	"go-admin/internal/service"
	"go-admin/pkg/core"
	"net/http"

	"github.com/gin-gonic/gin"
)

type NoticeHandler struct {
	svc *service.NoticeService
}

func NewNoticeHandler(svc *service.NoticeService) *NoticeHandler {
	return &NoticeHandler{svc: svc}
}

func (h *NoticeHandler) CreateNotice(c *gin.Context) {
	var req dto.CreateNoticeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		core.FailByError(c, core.ParseBindError(err))
		return
	}

	userID := c.GetInt64("user_id")
	if err := h.svc.CreateNotice(userID, req.Title, req.Content); err != nil {
		core.Fail(c, http.StatusInternalServerError, "创建通知失败")
		return
	}

	core.Success(c, "创建成功")
}
```

### 6. `internal/router/router.go`

最后接路由。

示例：

```go
auth.POST("/notice/create", container.NoticeHandler.CreateNotice)
auth.GET("/notice/list", container.NoticeHandler.GetNoticeList)
auth.PUT("/notice/:id/read", container.NoticeHandler.MarkRead)
```

### 7. `internal/container/container.go`

如果是新模块，别忘了把依赖注入接上。

示例：

```go
noticeDAO := dao.NewGormNoticeDAO(db)
noticeService := service.NewNoticeService(noticeDAO)
noticeHandler := handler.NewNoticeHandler(noticeService)
```

并把它挂到 `Container` 结构体中。

## 最小可用模板

如果只是快速加一个普通 CRUD 功能，最少需要这些文件：

```text
internal/domain/model/xxx.go
internal/dto/xxx_dto.go
internal/dao/xxx_dao.go
internal/service/xxx_service.go
internal/handler/xxx_handler.go
```

然后再改两个地方：

```text
internal/container/container.go
internal/router/router.go
```

## 推荐代码骨架

新增模块时，可以直接按这个顺序写：

### 第一步：建 model

```go
type XXX struct {
	ID int64 `gorm:"primaryKey"`
}
```

### 第二步：建 dto

```go
type CreateXXXRequest struct {
	Name string `json:"name" binding:"required"`
}
```

### 第三步：建 dao 接口

```go
type XXXDAO interface {
	Create(x *model.XXX) error
}
```

### 第四步：建 service

```go
type XXXService struct {
	dao dao.XXXDAO
}
```

### 第五步：建 handler

```go
type XXXHandler struct {
	svc *service.XXXService
}
```

### 第六步：挂 container

```go
xxxDAO := dao.NewGormXXXDAO(db)
xxxService := service.NewXXXService(xxxDAO)
xxxHandler := handler.NewXXXHandler(xxxService)
```

### 第七步：注册路由

```go
auth.POST("/xxx/create", container.XXXHandler.Create)
```

## 分层边界速查表

| 需求 | 应该放哪 |
| --- | --- |
| 绑定 JSON 参数 | `handler` |
| 参数校验标签 | `dto` |
| 默认角色、默认状态 | `service` |
| 密码加密 | `service` |
| 查询数据库 | `dao` |
| Redis 缓存读写 | `dao` 或 `service` |
| Gorm 模型定义 | `domain/model` |
| 路由 URL | `router` |
| JWT / 限流 / 跨域 | `middleware` |

## 常见错误

### 1. 把 model 当 dto 用

不建议这样做：

```go
var req model.User
```

原因：

- 数据库字段和接口字段关注点不同
- 容易把不该暴露的字段直接暴露出去
- 后面字段一改，会互相牵连

正确做法：

- 入参走 `dto`
- 落库走 `model`

### 2. 在 handler 里写业务逻辑

不建议这样做：

- handler 里判断一堆权限
- handler 里拼装复杂状态
- handler 里直接查多个表

正确做法：

- handler 只负责接和回
- 业务流程放到 service

### 3. 在 dao 里写业务规则

不建议在 dao 里写这种逻辑：

- “如果用户不是管理员就不给查”
- “如果已经点赞就取消点赞”

这些都属于业务规则，应该放在 `service`。

### 4. service 直接依赖 Gin

不建议这样做：

```go
func (s *XXXService) Create(c *gin.Context) {}
```

正确做法：

```go
func (s *XXXService) Create(userID int64, name string) error {}
```

## 新增功能自检清单

每次加完一个功能，可以快速对照：

- 是否有 `model`
- 是否有 `dto`
- 是否有 `dao`
- 是否有 `service`
- 是否有 `handler`
- 是否已经注入 `container`
- 是否已经注册 `router`
- 是否需要自动迁移
- 是否需要鉴权
- 是否需要限流
- 是否需要缓存
- 是否需要补测试

## 推荐实践
对当前这个项目，建议继续坚持：

- 每个业务模块独立一组 `handler/service/dao/dto/model`
- 共享小工具优先放 `pkg/core` 或 `pkg/utils`
- 共享业务辅助函数优先放 `internal/service`
- 

## 最简流程

```text
定 model -> 定 dto -> 写 dao -> 写 service -> 写 handler -> 接 router -> 注入 container
```
