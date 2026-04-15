package api

import (
	"go-admin/core"
	"go-admin/internal/repository"
	"go-admin/model"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// SocialAPI 封装社交相关的处理器，持有依赖
type SocialAPI struct {
	socialRepo repository.SocialRepository // 注入的Repository接口
}

// NewSocialAPI 构造函数：注入SocialRepository依赖
func NewSocialAPI(socialRepo repository.SocialRepository) *SocialAPI {
	return &SocialAPI{socialRepo: socialRepo}
}

// FollowUser 关注/取消关注用户
func (api *SocialAPI) FollowUser(c *gin.Context) {
	toUID, _ := strconv.Atoi(c.Param("uid"))
	fromUID := c.GetUint("userID")

	if uint(toUID) == fromUID {
		core.Fail(c, http.StatusBadRequest, "不能关注自己")
		return
	}

	// 检查目标用户是否存在
	toUser, err := api.socialRepo.FindUserByID(uint(toUID))
	if err != nil {
		core.Fail(c, http.StatusNotFound, "目标用户不存在")
		return
	}

	// 使用依赖注入的Repository查询关注状态
	existing, err := api.socialRepo.FindFollowRelation(fromUID, uint(toUID))

	if err == nil {
		// 已关注，取消关注
		if err := api.socialRepo.DeleteFollowRelation(existing); err != nil {
			core.Fail(c, http.StatusInternalServerError, "取消关注失败")
			return
		}

		core.Success(c, gin.H{
			"action":  "unfollow",
			"message": "已取消关注",
			"target_user": gin.H{
				"id":       toUser.ID,
				"username": toUser.Username,
			},
		})
		return
	}

	// 未关注，添加关注
	newRelation := &model.UserRelation{
		FromUID: fromUID,
		ToUID:   uint(toUID),
		Type:    "follow",
	}

	if err := api.socialRepo.CreateFollowRelation(newRelation); err != nil {
		core.Fail(c, http.StatusInternalServerError, "关注失败")
		return
	}

	core.Success(c, gin.H{
		"action":  "follow",
		"message": "关注成功",
		"target_user": gin.H{
			"id":       toUser.ID,
			"username": toUser.Username,
		},
	})
}

// BlockUser 拉黑/取消拉黑用户
func (api *SocialAPI) BlockUser(c *gin.Context) {
	toUID, _ := strconv.Atoi(c.Param("uid"))
	fromUID := c.GetUint("userID")

	if uint(toUID) == fromUID {
		core.Fail(c, http.StatusBadRequest, "不能拉黑自己")
		return
	}

	// 检查目标用户是否存在
	toUser, err := api.socialRepo.FindUserByID(uint(toUID))
	if err != nil {
		core.Fail(c, http.StatusNotFound, "目标用户不存在")
		return
	}

	// 使用依赖注入的Repository查询拉黑状态
	existing, err := api.socialRepo.FindBlockRelation(fromUID, uint(toUID))

	if err == nil {
		// 已拉黑，取消拉黑
		if err := api.socialRepo.DeleteBlockRelation(existing); err != nil {
			core.Fail(c, http.StatusInternalServerError, "取消拉黑失败")
			return
		}

		core.Success(c, gin.H{
			"action":  "unblock",
			"message": "已取消拉黑",
			"target_user": gin.H{
				"id":       toUser.ID,
				"username": toUser.Username,
			},
		})
		return
	}

	// 未拉黑，添加拉黑
	newRelation := &model.UserRelation{
		FromUID: fromUID,
		ToUID:   uint(toUID),
		Type:    "block",
	}

	if err := api.socialRepo.CreateBlockRelation(newRelation); err != nil {
		core.Fail(c, http.StatusInternalServerError, "拉黑失败")
		return
	}

	core.Success(c, gin.H{
		"action":  "block",
		"message": "拉黑成功",
		"target_user": gin.H{
			"id":       toUser.ID,
			"username": toUser.Username,
		},
	})
}

// GetRelationStatus 获取关系状态
func (api *SocialAPI) GetRelationStatus(c *gin.Context) {
	fromUID := c.GetUint("userID")
	toUID, _ := strconv.Atoi(c.Param("uid"))

	// 使用依赖注入的Repository获取关系状态
	status, err := api.socialRepo.GetRelationStatus(fromUID, uint(toUID))
	if err != nil {
		core.Fail(c, http.StatusInternalServerError, "获取关系状态失败")
		return
	}

	core.Success(c, status)
}

// GetFollowList 获取关注列表
func (api *SocialAPI) GetFollowList(c *gin.Context) {
	userID := c.GetUint("userID")

	// 使用依赖注入的Repository获取关注列表
	following, err := api.socialRepo.GetFollowing(userID)
	if err != nil {
		core.Fail(c, http.StatusInternalServerError, "获取关注列表失败")
		return
	}

	// 构建响应数据
	result := make([]gin.H, 0)
	for _, relation := range following {
		user, err := api.socialRepo.FindUserByID(relation.ToUID)
		if err == nil {
			result = append(result, gin.H{
				"id":         user.ID,
				"username":   user.Username,
				"avatar":     "", // User模型中没有Avatar字段
				"created_at": relation.CreatedAt,
			})
		}
	}

	core.Success(c, gin.H{
		"following": result,
		"count":     len(result),
	})
}

// GetFollowerList 获取粉丝列表
func (api *SocialAPI) GetFollowerList(c *gin.Context) {
	userID := c.GetUint("userID")

	// 使用依赖注入的Repository获取粉丝列表
	followers, err := api.socialRepo.GetFollowers(userID)
	if err != nil {
		core.Fail(c, http.StatusInternalServerError, "获取粉丝列表失败")
		return
	}

	// 构建响应数据
	result := make([]gin.H, 0)
	for _, relation := range followers {
		user, err := api.socialRepo.FindUserByID(relation.FromUID)
		if err == nil {
			result = append(result, gin.H{
				"id":         user.ID,
				"username":   user.Username,
				"avatar":     "", // User模型中没有Avatar字段
				"created_at": relation.CreatedAt,
			})
		}
	}

	core.Success(c, gin.H{
		"followers": result,
		"count":     len(result),
	})
}

// GetBlockList 获取拉黑列表
func (api *SocialAPI) GetBlockList(c *gin.Context) {
	userID := c.GetUint("userID")

	// 使用依赖注入的Repository获取拉黑列表
	blockedUsers, err := api.socialRepo.GetBlockedUsers(userID)
	if err != nil {
		core.Fail(c, http.StatusInternalServerError, "获取拉黑列表失败")
		return
	}

	// 构建响应数据
	result := make([]gin.H, 0)
	for _, relation := range blockedUsers {
		user, err := api.socialRepo.FindUserByID(relation.ToUID)
		if err == nil {
			result = append(result, gin.H{
				"id":         user.ID,
				"username":   user.Username,
				"avatar":     "", // User模型中没有Avatar字段
				"created_at": relation.CreatedAt,
			})
		}
	}

	core.Success(c, gin.H{
		"blocked_users": result,
		"count":         len(result),
	})
}
