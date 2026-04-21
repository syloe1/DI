package service

import (
	"net/http"
	"strconv"

	"go-admin/internal/dao"
	"go-admin/internal/domain/model"
	"go-admin/pkg/core"

	"gorm.io/gorm"
)

type SocialTargetUser struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
}

type SocialActionResult struct {
	Action     string           `json:"action"`
	Message    string           `json:"message"`
	TargetUser SocialTargetUser `json:"target_user"`
}

type SocialUserItem struct {
	ID        uint        `json:"id"`
	Username  string      `json:"username"`
	Avatar    string      `json:"avatar"`
	CreatedAt interface{} `json:"created_at"`
}

type SocialListResult struct {
	ItemsKey string           `json:"-"`
	Items    []SocialUserItem `json:"items"`
	Count    int              `json:"count"`
}

type SocialService struct {
	socialRepo dao.SocialRepository
}

func NewSocialService(socialRepo dao.SocialRepository) *SocialService {
	return &SocialService{socialRepo: socialRepo}
}

func (s *SocialService) FollowUser(fromUID uint, toUIDStr string) (*SocialActionResult, error) {
	toUID, err := strconv.Atoi(toUIDStr)
	if err != nil {
		return nil, core.NewBizError(http.StatusBadRequest, "invalid user id")
	}
	if uint(toUID) == fromUID {
		return nil, core.NewBizError(http.StatusBadRequest, "cannot follow yourself")
	}

	toUser, err := s.socialRepo.FindUserByID(uint(toUID))
	if err != nil {
		return nil, core.NewBizError(http.StatusNotFound, "target user not found")
	}

	following := false
	err = s.socialRepo.Transaction(func(repoWithTx dao.SocialRepository) error {
		existing, err := repoWithTx.FindFollowRelation(fromUID, uint(toUID))
		if err != nil {
			if err != gorm.ErrRecordNotFound {
				return err
			}

			newRelation := &model.UserRelation{
				FromUID: fromUID,
				ToUID:   uint(toUID),
				Type:    "follow",
			}
			if err := repoWithTx.CreateFollowRelation(newRelation); err != nil {
				return err
			}
			following = true
			return nil
		}

		if err := repoWithTx.DeleteFollowRelation(existing); err != nil {
			return err
		}
		following = false
		return nil
	})
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "follow action failed")
	}

	result := &SocialActionResult{
		TargetUser: SocialTargetUser{
			ID:       toUser.ID,
			Username: toUser.Username,
		},
	}
	if following {
		result.Action = "follow"
		result.Message = "follow success"
		return result, nil
	}

	result.Action = "unfollow"
	result.Message = "unfollow success"
	return result, nil
}

func (s *SocialService) BlockUser(fromUID uint, toUIDStr string) (*SocialActionResult, error) {
	toUID, err := strconv.Atoi(toUIDStr)
	if err != nil {
		return nil, core.NewBizError(http.StatusBadRequest, "invalid user id")
	}
	if uint(toUID) == fromUID {
		return nil, core.NewBizError(http.StatusBadRequest, "cannot block yourself")
	}

	toUser, err := s.socialRepo.FindUserByID(uint(toUID))
	if err != nil {
		return nil, core.NewBizError(http.StatusNotFound, "target user not found")
	}

	blocking := false
	err = s.socialRepo.Transaction(func(repoWithTx dao.SocialRepository) error {
		existing, err := repoWithTx.FindBlockRelation(fromUID, uint(toUID))
		if err != nil {
			if err != gorm.ErrRecordNotFound {
				return err
			}
			newRelation := &model.UserRelation{
				FromUID: fromUID,
				ToUID:   uint(toUID),
				Type:    "block",
			}
			if err := repoWithTx.CreateBlockRelation(newRelation); err != nil {
				return err
			}
			blocking = true
			return nil
		}
		if err := repoWithTx.DeleteBlockRelation(existing); err != nil {
			return err
		}
		blocking = false
		return nil
	})
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "block action failed")
	}

	result := &SocialActionResult{
		TargetUser: SocialTargetUser{
			ID:       toUser.ID,
			Username: toUser.Username,
		},
	}
	if blocking {
		result.Action = "block"
		result.Message = "block success"
		return result, nil
	}

	result.Action = "unblock"
	result.Message = "unblock success"
	return result, nil
}

func (s *SocialService) GetRelationStatus(fromUID uint, toUIDStr string) (map[string]interface{}, error) {
	toUID, err := strconv.Atoi(toUIDStr)
	if err != nil {
		return nil, core.NewBizError(http.StatusBadRequest, "invalid user id")
	}

	status, err := s.socialRepo.GetRelationStatus(fromUID, uint(toUID))
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "get relation status failed")
	}
	return status, nil
}

func (s *SocialService) GetFollowList(userID uint) (map[string]interface{}, error) {
	following, err := s.socialRepo.GetFollowing(userID)
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "get following list failed")
	}

	result := make([]map[string]interface{}, 0)
	for _, relation := range following {
		user, err := s.socialRepo.FindUserByID(relation.ToUID)
		if err == nil {
			result = append(result, map[string]interface{}{
				"id":         user.ID,
				"username":   user.Username,
				"avatar":     "",
				"created_at": relation.CreatedAt,
			})
		}
	}

	return map[string]interface{}{
		"following": result,
		"count":     len(result),
	}, nil
}

func (s *SocialService) GetFollowerList(userID uint) (map[string]interface{}, error) {
	followers, err := s.socialRepo.GetFollowers(userID)
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "get follower list failed")
	}

	result := make([]map[string]interface{}, 0)
	for _, relation := range followers {
		user, err := s.socialRepo.FindUserByID(relation.FromUID)
		if err == nil {
			result = append(result, map[string]interface{}{
				"id":         user.ID,
				"username":   user.Username,
				"avatar":     "",
				"created_at": relation.CreatedAt,
			})
		}
	}

	return map[string]interface{}{
		"followers": result,
		"count":     len(result),
	}, nil
}

func (s *SocialService) GetBlockList(userID uint) (map[string]interface{}, error) {
	blockedUsers, err := s.socialRepo.GetBlockedUsers(userID)
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "get block list failed")
	}

	result := make([]map[string]interface{}, 0)
	for _, relation := range blockedUsers {
		user, err := s.socialRepo.FindUserByID(relation.ToUID)
		if err == nil {
			result = append(result, map[string]interface{}{
				"id":         user.ID,
				"username":   user.Username,
				"avatar":     "",
				"created_at": relation.CreatedAt,
			})
		}
	}

	return map[string]interface{}{
		"blocked_users": result,
		"count":         len(result),
	}, nil
}
