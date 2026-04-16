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

type PostAPI struct {
	postRepo          repository.PostRepository
	interactExtension postInteractExtension
}

func NewPostAPI(postRepo repository.PostRepository) *PostAPI {
	return &PostAPI{postRepo: postRepo}
}

type postInteractExtension interface {
	GetInteractCount(c *gin.Context)
	ToggleLike(c *gin.Context)
	ToggleDislike(c *gin.Context)
	ToggleCollect(c *gin.Context)
	Share(c *gin.Context)
	GetInteractStatus(c *gin.Context)
}

func (api *PostAPI) SetInteractExtension(extension postInteractExtension) {
	api.interactExtension = extension
}

func (api *PostAPI) CreatePost(c *gin.Context) {
	var req struct {
		Title     string  `json:"title"`
		Content   string  `json:"content"`
		IsPublic  bool    `json:"is_public"`
		Status    string  `json:"status"`
		PublishAt *string `json:"publish_at"`
		Topics    string  `json:"topics"`
		Images    string  `json:"images"`
	}

	if err := c.ShouldBindJSON(&req); err != nil || req.Title == "" || req.Content == "" {
		core.Fail(c, http.StatusBadRequest, "标题和内容不能为空")
		return
	}

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

	if err := api.postRepo.Create(&post); err != nil {
		core.Fail(c, http.StatusInternalServerError, "创建帖子失败")
		return
	}

	core.Success(c, post)
}

func (api *PostAPI) GetPostList(c *gin.Context) {
	topic := c.Query("topic")
	posts, err := api.postRepo.FindPublicByTopic(topic)
	if err != nil {
		core.Fail(c, http.StatusInternalServerError, "查询帖子失败: "+err.Error())
		return
	}

	core.Success(c, posts)
}

func (api *PostAPI) GetPost(c *gin.Context) {
	id := c.Param("id")
	currentUserID := c.GetUint("userID")
	currentUserRole := c.GetString("role")

	post, err := api.postRepo.FindByID(id)
	if err != nil {
		core.Fail(c, http.StatusNotFound, "帖子不存在")
		return
	}

	if !post.IsPublic && post.UserID != currentUserID && currentUserRole != "admin" && currentUserRole != "superadmin" {
		core.Fail(c, http.StatusForbidden, "无权查看此帖子")
		return
	}

	core.Success(c, post)
}

func (api *PostAPI) DeletePost(c *gin.Context) {
	id := c.Param("id")
	currentUserID := c.GetUint("userID")
	currentUserRole := c.GetString("role")

	post, err := api.postRepo.FindByID(id)
	if err != nil {
		core.Fail(c, http.StatusNotFound, "帖子不存在")
		return
	}

	if post.UserID != currentUserID && currentUserRole != "admin" && currentUserRole != "superadmin" {
		core.Fail(c, http.StatusForbidden, "没有权限删除此帖子")
		return
	}

	if err := api.postRepo.Delete(post); err != nil {
		core.Fail(c, http.StatusInternalServerError, "删除帖子失败: "+err.Error())
		return
	}

	core.SuccessWithMessage(c, "删除成功", nil)
}

func (api *PostAPI) UpdatePost(c *gin.Context) {
	id := c.Param("id")
	currentUserID := c.GetUint("userID")

	post, err := api.postRepo.FindByID(id)
	if err != nil {
		core.Fail(c, http.StatusNotFound, "帖子不存在")
		return
	}

	if post.UserID != currentUserID {
		core.Fail(c, http.StatusForbidden, "无权修改他人帖子")
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
	_ = c.ShouldBindJSON(&req)

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

	if err := api.postRepo.Update(post, updates); err != nil {
		core.Fail(c, http.StatusInternalServerError, "修改帖子失败: "+err.Error())
		return
	}

	core.SuccessWithMessage(c, "修改成功", nil)
}

func (api *PostAPI) GetMyPostList(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		core.Fail(c, http.StatusUnauthorized, "未获取到用户信息")
		return
	}

	userIDUint, ok := userID.(uint)
	if !ok {
		core.Fail(c, http.StatusBadRequest, "用户ID格式错误")
		return
	}

	posts, err := api.postRepo.FindByUserID(userIDUint)
	if err != nil {
		core.Fail(c, http.StatusInternalServerError, "获取我的帖子失败: "+err.Error())
		return
	}

	core.SuccessWithMessage(c, "获取我的帖子成功", posts)
}

func (api *PostAPI) GetUserPosts(c *gin.Context) {
	userID := c.Param("id")
	posts, err := api.postRepo.FindPublicByUserID(userID)
	if err != nil {
		core.Fail(c, http.StatusInternalServerError, "获取用户帖子失败: "+err.Error())
		return
	}

	core.SuccessWithMessage(c, "获取用户帖子成功", posts)
}

func (api *PostAPI) GetUserLikedPosts(c *gin.Context) {
	userID := c.Param("id")
	posts, err := api.postRepo.FindLikedByUserID(userID)
	if err != nil {
		core.Fail(c, http.StatusInternalServerError, "获取用户点赞帖子失败: "+err.Error())
		return
	}

	core.SuccessWithMessage(c, "获取用户点赞帖子成功", posts)
}

func (api *PostAPI) GetUserCollectedPosts(c *gin.Context) {
	userID := c.Param("id")
	posts, err := api.postRepo.FindCollectedByUserID(userID)
	if err != nil {
		core.Fail(c, http.StatusInternalServerError, "获取用户收藏帖子失败: "+err.Error())
		return
	}

	core.SuccessWithMessage(c, "获取用户收藏帖子成功", posts)
}

func (api *PostAPI) GetInteractCount(c *gin.Context) {
	if api.interactExtension == nil {
		core.Fail(c, http.StatusNotImplemented, "互动统计功能暂未实现")
		return
	}
	api.interactExtension.GetInteractCount(c)
}

func (api *PostAPI) ToggleLike(c *gin.Context) {
	if api.interactExtension == nil {
		core.Fail(c, http.StatusNotImplemented, "点赞功能暂未实现")
		return
	}
	api.interactExtension.ToggleLike(c)
}

func (api *PostAPI) ToggleDislike(c *gin.Context) {
	if api.interactExtension == nil {
		core.Fail(c, http.StatusNotImplemented, "点踩功能暂未实现")
		return
	}
	api.interactExtension.ToggleDislike(c)
}

func (api *PostAPI) ToggleCollect(c *gin.Context) {
	if api.interactExtension == nil {
		core.Fail(c, http.StatusNotImplemented, "收藏功能暂未实现")
		return
	}
	api.interactExtension.ToggleCollect(c)
}

func (api *PostAPI) Share(c *gin.Context) {
	if api.interactExtension == nil {
		core.Fail(c, http.StatusNotImplemented, "分享功能暂未实现")
		return
	}
	api.interactExtension.Share(c)
}

func (api *PostAPI) GetInteractStatus(c *gin.Context) {
	if api.interactExtension == nil {
		core.Fail(c, http.StatusNotImplemented, "互动状态功能暂未实现")
		return
	}
	api.interactExtension.GetInteractStatus(c)
}

func extractTopics(content, extra string) string {
	topicSet := map[string]bool{}
	words := strings.Fields(content)
	for _, w := range words {
		if strings.HasPrefix(w, "#") {
			tag := strings.TrimRight(w[1:], ",，。！？.!?")
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
	res := make([]string, 0, len(topicSet))
	for t := range topicSet {
		res = append(res, t)
	}
	return strings.Join(res, ",")
}
