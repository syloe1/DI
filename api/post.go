package api

import (
	"go-admin/core"
	"go-admin/internal/repository"
	"go-admin/model"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// PostAPI 封装帖子相关的处理器，持有依赖
type PostAPI struct {
	postRepo repository.PostRepository // 注入的Repository接口
}

// NewPostAPI 构造函数：注入PostRepository依赖
func NewPostAPI(postRepo repository.PostRepository) *PostAPI {
	return &PostAPI{postRepo: postRepo}
}

// -------------- 以下是改造后的处理器方法（替换原全局DB为注入的postRepo）--------------
func (api *PostAPI) CreatePost(c *gin.Context) {
	var req struct {
		Title     string  `json:"title"`
		Content   string  `json:"content"`
		IsPublic  bool    `json:"is_public"`
		Status    string  `json:"status"`
		PublishAt *string `json:"publish_at"` // RFC3339格式
		Topics    string  `json:"topics"`
		Images    string  `json:"images"`
	}

	// 参数校验
	if err := c.ShouldBindJSON(&req); err != nil || req.Title == "" || req.Content == "" {
		core.Fail(c, http.StatusBadRequest, "标题和内容不能为空")
		return
	}

	// 构建帖子对象
	post := model.Post{
		Title:    req.Title,
		Content:  req.Content,
		IsPublic: req.IsPublic,
		Status:   req.Status,
		Topics:   extractTopics(req.Content, req.Topics),
		UserID:   c.GetUint("userID"),
		Username: c.GetString("username"),
		Images:   req.Images,
	}

	// 处理定时发布
	if req.PublishAt != nil && *req.PublishAt != "" {
		t, err := time.Parse(time.RFC3339, *req.PublishAt)
		if err == nil {
			post.PublishAt = &t
			post.Status = model.PostStatusScheduled
		}
	}

	if post.Status == "" {
		post.Status = model.PostStatusPublished
	}

	// 使用依赖注入的Repository
	if err := api.postRepo.Create(&post); err != nil {
		core.Fail(c, http.StatusInternalServerError, "创建帖子失败")
		return
	}

	core.Success(c, post)
}

func (api *PostAPI) GetPostList(c *gin.Context) {
	topic := c.Query("topic")

	// 使用注入的Repository查询
	posts, err := api.postRepo.FindPublicByTopic(topic)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询帖子失败：" + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "data": posts})
}

func (api *PostAPI) GetPost(c *gin.Context) {
	id := c.Param("id")
	currentUserID := c.GetUint("userID")
	currentUserRole := c.GetString("role")

	// 使用注入的Repository查询
	post, err := api.postRepo.FindByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "帖子不存在"})
		return
	}

	// 权限检查（逻辑不变）
	if !post.IsPublic && post.UserID != currentUserID && currentUserRole != "admin" && currentUserRole != "superadmin" {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "无权查看此帖子"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "data": post})
}

func (api *PostAPI) DeletePost(c *gin.Context) {
	id := c.Param("id")
	currentUserID := c.GetUint("userID")
	currentUserRole := c.GetString("role")

	// 使用注入的Repository查询
	post, err := api.postRepo.FindByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "帖子不存在"})
		return
	}

	// 权限检查（逻辑不变）
	if post.UserID != currentUserID && currentUserRole != "admin" && currentUserRole != "superadmin" {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "没有权限删除此帖子"})
		return
	}

	// 使用注入的Repository删除
	if err := api.postRepo.Delete(post); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "删除帖子失败：" + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "删除成功"})
}

func (api *PostAPI) UpdatePost(c *gin.Context) {
	id := c.Param("id")
	currentUserID := c.GetUint("userID")

	// 使用注入的Repository查询
	post, err := api.postRepo.FindByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "帖子不存在"})
		return
	}

	// 权限检查（逻辑不变）
	if post.UserID != currentUserID {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "无权修改他人帖子"})
		return
	}

	var req struct {
		Title     string  `json:"title"`
		Content   string  `json:"content"`
		IsPublic  *bool   `json:"is_public"`
		Status    string  `json:"status"`
		PublishAt *string `json:"publish_at"`
		Topics    string  `json:"topics"`
		Images    string  `json:"images"`
	}
	c.ShouldBindJSON(&req)

	updates := map[string]interface{}{}
	if req.Title != "" {
		updates["title"] = req.Title
	}
	if req.Content != "" {
		updates["content"] = req.Content
		updates["topics"] = extractTopics(req.Content, req.Topics)
	}
	if req.IsPublic != nil {
		updates["is_public"] = *req.IsPublic
	}
	if req.Status != "" {
		updates["status"] = req.Status
	}
	if req.PublishAt != nil {
		if *req.PublishAt == "" {
			updates["publish_at"] = nil
		} else {
			t, err := time.Parse(time.RFC3339, *req.PublishAt)
			if err == nil {
				updates["publish_at"] = t
				updates["status"] = model.PostStatusScheduled
			}
		}
	}
	if req.Images != "" {
		updates["images"] = req.Images
	}

	// 使用注入的Repository更新
	if err := api.postRepo.Update(post, updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "修改帖子失败：" + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "修改成功"})
}

func (api *PostAPI) GetMyPostList(c *gin.Context) {
	userId, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code": 401,
			"msg":  "未获取到用户信息",
		})
		return
	}

	// 类型断言（原逻辑）
	userIDUint, ok := userId.(uint)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "用户ID格式错误"})
		return
	}

	// 使用注入的Repository查询
	posts, err := api.postRepo.FindByUserID(userIDUint)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "获取我的帖子失败：" + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "获取我的帖子成功",
		"data": posts,
	})
}

func (api *PostAPI) GetUserPosts(c *gin.Context) {
	userID := c.Param("id")

	// 使用注入的Repository查询
	posts, err := api.postRepo.FindPublicByUserID(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "获取用户帖子失败：" + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "获取用户帖子成功",
		"data": posts,
	})
}

func (api *PostAPI) GetUserLikedPosts(c *gin.Context) {
	userID := c.Param("id")

	// 使用注入的Repository查询
	posts, err := api.postRepo.FindLikedByUserID(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "获取用户点赞帖子失败：" + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "获取用户点赞帖子成功",
		"data": posts,
	})
}

func (api *PostAPI) GetUserCollectedPosts(c *gin.Context) {
	userID := c.Param("id")

	// 使用注入的Repository查询
	posts, err := api.postRepo.FindCollectedByUserID(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "获取用户收藏帖子失败：" + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "获取用户收藏帖子成功",
		"data": posts,
	})
}

// 工具函数（逻辑不变）
func extractTopics(content, extra string) string {
	topicSet := map[string]bool{}
	words := strings.Fields(content) // 按空格切分
	for _, w := range words {
		if strings.HasPrefix(w, "#") { // 找#开头的
			tag := strings.TrimRight(w[1:], ",。！？,.!?") // 去除第一个#， 还有后面的标点
			if tag != "" {
				topicSet[tag] = true
			}
		}
	}
	if extra != "" {
		for _, t := range strings.Split(extra, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				topicSet[t] = true
			}
		}
	}
	res := []string{}
	for t := range topicSet {
		res = append(res, t)
	}
	return strings.Join(res, ",")
}
