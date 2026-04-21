package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go-admin/internal/dao"
	"go-admin/internal/domain/model"
	"go-admin/pkg/core"
)

const (
	postDuplicateSubmitTTL = 5 * time.Second
	postHotRankKey         = "post:hot_rank"
	postHotScoreView       = 1
)

type HotPostItem struct {
	model.Post
	Score float64 `json:"score"`
}

type PostListResult struct {
	List     []model.Post `json:"list"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
	Total    int64        `json:"total"`
}

type PostService struct {
	postRepo dao.PostRepository
	cache    dao.UserCache
	ctx      context.Context
}

func NewPostService(postRepo dao.PostRepository, cache dao.UserCache, ctx context.Context) *PostService {
	return &PostService{
		postRepo: postRepo,
		cache:    cache,
		ctx:      ctx,
	}
}

func (s *PostService) CreatePost(userID uint, username string, title string, content string, isPublic bool, status string, publishAt *string, topics string, images string) (*model.Post, error) {
	post := model.Post{
		Title:    title,
		Content:  content,
		IsPublic: isPublic,
		Status:   status,
		Topics:   extractTopics(content, topics),
		UserID:   userID,
		Username: username,
		Images:   images,
	}

	if publishAt != nil && *publishAt != "" {
		t, err := time.Parse(time.RFC3339, *publishAt)
		if err == nil {
			post.PublishAt = &t
			post.Status = model.PostStatusScheduled
		}
	}

	if post.Status == "" {
		post.Status = model.PostStatusPublished
	}

	if err := s.postRepo.Create(&post); err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "create post failed")
	}

	s.invalidatePostListCaches(post.Topics)
	return &post, nil
}

func (s *PostService) GetPostList(topic string, sort string, page int, pageSize int) (*PostListResult, error) {
	page, pageSize = normalizePagination(page, pageSize, 1, 10)
	if sort == "" {
		sort = "time"
	}
	strategy := dao.ResolvePostSortStrategy(sort)
	key := s.postListCacheKey(topic, strategy.Name(), page, pageSize)

	if val, err := s.cache.Get(s.ctx, key); err == nil {
		if val == cacheNullValue {
			return &PostListResult{List: []model.Post{}, Page: page, PageSize: pageSize, Total: 0}, nil
		}

		var payload struct {
			List  []model.Post `json:"list"`
			Total int64        `json:"total"`
		}
		if err := json.Unmarshal([]byte(val), &payload); err == nil {
			return &PostListResult{List: payload.List, Page: page, PageSize: pageSize, Total: payload.Total}, nil
		}
	}

	lockKey := "lock:" + key
	locked, err := s.cache.SetNX(s.ctx, lockKey, "1", cacheLockTTL)
	if err == nil && !locked {
		if val, ok := spinWaitCache(s.cache, s.ctx, key); ok {
			if val == cacheNullValue {
				return &PostListResult{List: []model.Post{}, Page: page, PageSize: pageSize, Total: 0}, nil
			}

			var payload struct {
				List  []model.Post `json:"list"`
				Total int64        `json:"total"`
			}
			if err := json.Unmarshal([]byte(val), &payload); err == nil {
				return &PostListResult{List: payload.List, Page: page, PageSize: pageSize, Total: payload.Total}, nil
			}
		}
	}
	if locked {
		defer s.cache.Del(s.ctx, lockKey)
	}

	posts, total, err := s.postRepo.FindPublicPage(topic, page, pageSize, strategy)
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "get post list failed")
	}

	if len(posts) == 0 {
		_ = s.cache.Set(s.ctx, key, cacheNullValue, cacheNullTTL)
		return &PostListResult{List: []model.Post{}, Page: page, PageSize: pageSize, Total: 0}, nil
	}

	if data, err := json.Marshal(map[string]interface{}{"list": posts, "total": total}); err == nil {
		_ = s.cache.Set(s.ctx, key, string(data), jitterTTL(defaultCacheTTL))
	}

	return &PostListResult{List: posts, Page: page, PageSize: pageSize, Total: total}, nil
}

func (s *PostService) GetHotPosts(limit int) ([]HotPostItem, error) {
	rankItems, err := s.cache.ZRevRangeWithScores(s.ctx, postHotRankKey, 0, int64(limit-1))
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "get hot posts failed")
	}

	if len(rankItems) == 0 {
		return []HotPostItem{}, nil
	}

	ids := make([]uint, 0, len(rankItems))
	scoreMap := make(map[uint]float64, len(rankItems))
	for _, rankItem := range rankItems {
		id, err := strconv.ParseUint(rankItem.Member, 10, 32)
		if err != nil {
			continue
		}
		uintID := uint(id)
		ids = append(ids, uintID)
		scoreMap[uintID] = rankItem.Score
	}

	posts, err := s.postRepo.FindByIDs(ids)
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "get hot posts failed")
	}

	postMap := make(map[uint]HotPostItem, len(posts))
	for _, post := range posts {
		if !post.IsPublic || post.Status != model.PostStatusPublished {
			continue
		}
		s.fillPostViewCount(&post)
		postMap[post.ID] = HotPostItem{
			Post:  post,
			Score: scoreMap[post.ID],
		}
	}

	orderedPosts := make([]HotPostItem, 0, len(ids))
	for _, id := range ids {
		post, ok := postMap[id]
		if !ok {
			continue
		}
		orderedPosts = append(orderedPosts, post)
	}

	return orderedPosts, nil
}

func (s *PostService) GetPost(id string, currentUserID uint, currentUserRole string) (*model.Post, error) {
	key := "post:" + id
	if val, err := s.cache.Get(s.ctx, key); err == nil {
		if val == cacheNullValue {
			return nil, core.NewBizError(http.StatusNotFound, "post not found")
		}
		var post model.Post
		if err := json.Unmarshal([]byte(val), &post); err == nil {
			if !post.IsPublic && post.UserID != currentUserID && currentUserRole != model.RoleAdmin && currentUserRole != model.RoleSuperAdmin {
				return nil, core.NewBizError(http.StatusForbidden, "no permission to view this post")
			}
			s.attachPostViewCount(&post)
			return &post, nil
		}
	}

	lockKey := "lock:" + key
	locked, err := s.cache.SetNX(s.ctx, lockKey, "1", cacheLockTTL)
	if err == nil && !locked {
		if val, ok := spinWaitCache(s.cache, s.ctx, key); ok {
			if val == cacheNullValue {
				return nil, core.NewBizError(http.StatusNotFound, "post not found")
			}
			var post model.Post
			if err := json.Unmarshal([]byte(val), &post); err == nil {
				if !post.IsPublic && post.UserID != currentUserID && currentUserRole != model.RoleAdmin && currentUserRole != model.RoleSuperAdmin {
					return nil, core.NewBizError(http.StatusForbidden, "no permission to view this post")
				}
				s.attachPostViewCount(&post)
				return &post, nil
			}
		}
	}
	if locked {
		defer s.cache.Del(s.ctx, lockKey)
	}

	post, err := s.postRepo.FindByID(id)
	if err != nil {
		_ = s.cache.Set(s.ctx, key, cacheNullValue, cacheNullTTL)
		return nil, core.NewBizError(http.StatusNotFound, "post not found")
	}
	if !post.IsPublic && post.UserID != currentUserID && currentUserRole != model.RoleAdmin && currentUserRole != model.RoleSuperAdmin {
		return nil, core.NewBizError(http.StatusForbidden, "no permission to view this post")
	}
	if data, err := json.Marshal(post); err == nil {
		_ = s.cache.Set(s.ctx, key, string(data), jitterTTL(defaultCacheTTL))
	}
	s.attachPostViewCount(post)
	return post, nil
}

func (s *PostService) DeletePost(id string, currentUserID uint, currentUserRole string) error {
	post, err := s.postRepo.FindByID(id)
	if err != nil {
		return core.NewBizError(http.StatusNotFound, "post not found")
	}

	if post.UserID != currentUserID && currentUserRole != model.RoleAdmin && currentUserRole != model.RoleSuperAdmin {
		return core.NewBizError(http.StatusForbidden, "no permission to delete this post")
	}

	if err := s.postRepo.Delete(post); err != nil {
		return core.NewBizError(http.StatusInternalServerError, "delete post failed")
	}

	s.invalidatePostCache(id, post.Topics)
	return nil
}

func (s *PostService) UpdatePost(id string, currentUserID uint, title string, content string, isPublic *bool, status string, publishAt *string, topics string, images string) error {
	post, err := s.postRepo.FindByID(id)
	if err != nil {
		return core.NewBizError(http.StatusNotFound, "post not found")
	}

	if post.UserID != currentUserID {
		return core.NewBizError(http.StatusForbidden, "no permission to update this post")
	}

	oldTopics := post.Topics
	updates := map[string]interface{}{}
	newTopics := oldTopics

	if title != "" {
		updates["title"] = title
	}
	if content != "" {
		newTopics = extractTopics(content, topics)
		updates["content"] = content
		updates["topics"] = newTopics
	}
	if isPublic != nil {
		updates["is_public"] = *isPublic
	}
	if status != "" {
		updates["status"] = status
	}
	if publishAt != nil {
		if *publishAt == "" {
			updates["publish_at"] = nil
		} else {
			t, err := time.Parse(time.RFC3339, *publishAt)
			if err == nil {
				updates["publish_at"] = t
				updates["status"] = model.PostStatusScheduled
			}
		}
	}
	if images != "" {
		updates["images"] = images
	}

	if err := s.postRepo.Update(post, updates); err != nil {
		return core.NewBizError(http.StatusInternalServerError, "update post failed")
	}

	s.invalidatePostCache(id, oldTopics, newTopics)
	return nil
}

func (s *PostService) GetMyPostList(userID uint) ([]model.Post, error) {
	posts, err := s.postRepo.FindByUserID(userID)
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "get my posts failed")
	}
	return posts, nil
}

func (s *PostService) GetUserPosts(userID string) ([]model.Post, error) {
	posts, err := s.postRepo.FindPublicByUserID(userID)
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "get user posts failed")
	}
	return posts, nil
}

func (s *PostService) GetUserLikedPosts(userID string) ([]model.Post, error) {
	posts, err := s.postRepo.FindLikedByUserID(userID)
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "get liked posts failed")
	}
	return posts, nil
}

func (s *PostService) GetUserCollectedPosts(userID string) ([]model.Post, error) {
	posts, err := s.postRepo.FindCollectedByUserID(userID)
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "get collected posts failed")
	}
	return posts, nil
}

func (s *PostService) DuplicateSubmitKey(userID uint, title string, content string) string {
	return fmt.Sprintf("duplicate:post:%d:%s", userID, hashPayload(title, content))
}

func (s *PostService) CheckDuplicateSubmit(userID uint, title string, content string) (bool, error) {
	key := s.DuplicateSubmitKey(userID, title, content)
	ok, err := s.cache.SetNX(s.ctx, key, "1", postDuplicateSubmitTTL)
	if err != nil {
		return false, err
	}
	return !ok, nil
}

func extractTopics(content string, extra string) string {
	topicSet := map[string]bool{}
	words := strings.Fields(content)
	for _, w := range words {
		if strings.HasPrefix(w, "#") {
			tag := strings.TrimRight(w[1:], ",锛屻€傦紒锛?!?")
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

func (s *PostService) postListCacheKey(topic string, sort string, page int, pageSize int) string {
	key := "post:list"
	if sort != "" {
		key += ":sort:" + sort
	}
	if topic != "" {
		key += ":topic:" + topic
	}
	key += fmt.Sprintf(":page:%d:size:%d", page, pageSize)
	return key
}

func (s *PostService) invalidatePostCache(id string, topics ...string) {
	keys := []string{"post:" + id}
	s.appendTopicCacheKeys(&keys, topics...)
	_ = s.cache.Del(s.ctx, keys...)
}

func (s *PostService) invalidatePostListCaches(topics ...string) {
	keys := []string{}
	s.appendTopicCacheKeys(&keys, topics...)
	_ = s.cache.Del(s.ctx, keys...)
}

func (s *PostService) appendTopicCacheKeys(keys *[]string, topics ...string) {
	for _, topicGroup := range topics {
		if topicGroup == "" {
			continue
		}
		for _, topic := range strings.Split(topicGroup, ",") {
			topic = strings.TrimSpace(topic)
			if topic != "" {
				*keys = append(*keys, "post:list:sort:time:topic:"+topic, "post:list:sort:hot:topic:"+topic)
			}
		}
	}
}

func normalizePagination(page int, pageSize int, defaultPage int, defaultPageSize int) (int, int) {
	if page <= 0 {
		page = defaultPage
	}
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	return page, pageSize
}

func (s *PostService) fillPostViewCount(post *model.Post) {
	if post == nil || post.ID == 0 {
		return
	}
	key := fmt.Sprintf("post:view:%d", post.ID)
	value, err := s.cache.Get(s.ctx, key)
	if err != nil {
		return
	}
	viewCount, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return
	}
	post.ViewCount = viewCount
}

func (s *PostService) attachPostViewCount(post *model.Post) {
	if post == nil || post.ID == 0 {
		return
	}
	key := fmt.Sprintf("post:view:%d", post.ID)
	viewCount, err := s.cache.Incr(s.ctx, key)
	if err != nil {
		return
	}
	post.ViewCount = viewCount
	_, _ = s.cache.ZIncrBy(s.ctx, postHotRankKey, postHotScoreView, fmt.Sprintf("%d", post.ID))
}
