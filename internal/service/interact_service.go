package service

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"go-admin/internal/dao"
	"go-admin/internal/domain/model"
	"go-admin/pkg/core"

	"gorm.io/gorm"
)

const (
	postHotScoreLike    = 5
	postHotScoreCollect = 8
	postHotScoreShare   = 10
)

type InteractActionResult struct {
	Status  bool   `json:"status,omitempty"`
	Message string `json:"message"`
}

type InteractService struct {
	interactRepo dao.InteractRepository
	cache        dao.UserCache
	ctx          context.Context
}

func NewInteractService(interactRepo dao.InteractRepository, cache dao.UserCache, ctx context.Context) *InteractService {
	return &InteractService{
		interactRepo: interactRepo,
		cache:        cache,
		ctx:          ctx,
	}
}

func (s *InteractService) ToggleLike(userID uint, postIDStr string) (*InteractActionResult, error) {
	postID, err := parsePostID(postIDStr)
	if err != nil {
		return nil, err
	}

	liked := false
	err = s.interactRepo.Transaction(func(repoWithTx dao.InteractRepository) error {
		like, err := repoWithTx.FindLike(userID, postID)
		if err != nil {
			if err != gorm.ErrRecordNotFound {
				return err
			}

			dislike, err := repoWithTx.FindDislike(userID, postID)
			if err == nil && dislike != nil {
				if err := repoWithTx.DeleteDislike(dislike); err != nil {
					return err
				}
			} else if err != nil && err != gorm.ErrRecordNotFound {
				return err
			}

			newLike := &model.Like{
				UserID: userID,
				PostID: postID,
			}
			if err := repoWithTx.CreateLike(newLike); err != nil {
				return err
			}

			liked = true
			return nil
		}

		if err := repoWithTx.DeleteLike(like); err != nil {
			return err
		}

		liked = false
		return nil
	})
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "toggle like failed")
	}

	if liked {
		s.bumpPostHotScore(postID, postHotScoreLike)
		return &InteractActionResult{Status: true, Message: "like success"}, nil
	}

	s.bumpPostHotScore(postID, -postHotScoreLike)
	return &InteractActionResult{Status: false, Message: "cancel like success"}, nil
}

func (s *InteractService) ToggleDislike(userID uint, postIDStr string) (*InteractActionResult, error) {
	postID, err := parsePostID(postIDStr)
	if err != nil {
		return nil, err
	}

	disliked := false
	err = s.interactRepo.Transaction(func(repoWithTx dao.InteractRepository) error {
		dislike, err := repoWithTx.FindDislike(userID, postID)
		if err != nil {
			if err != gorm.ErrRecordNotFound {
				return err
			}

			like, err := repoWithTx.FindLike(userID, postID)
			if err == nil && like != nil {
				if err := repoWithTx.DeleteLike(like); err != nil {
					return err
				}
			} else if err != nil && err != gorm.ErrRecordNotFound {
				return err
			}

			newDislike := &model.Dislike{
				UserID: userID,
				PostID: postID,
			}
			if err := repoWithTx.CreateDislike(newDislike); err != nil {
				return err
			}
			disliked = true
			return nil
		}

		if err := repoWithTx.DeleteDislike(dislike); err != nil {
			return err
		}
		disliked = false
		return nil
	})
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "toggle dislike failed")
	}

	if disliked {
		return &InteractActionResult{Status: true, Message: "dislike success"}, nil
	}
	return &InteractActionResult{Status: false, Message: "cancel dislike success"}, nil
}

func (s *InteractService) ToggleCollect(userID uint, postIDStr string) (*InteractActionResult, error) {
	postID, err := parsePostID(postIDStr)
	if err != nil {
		return nil, err
	}

	collected := false
	err = s.interactRepo.Transaction(func(repoWithTx dao.InteractRepository) error {
		collect, err := repoWithTx.FindCollect(userID, postID)
		if err != nil {
			if err != gorm.ErrRecordNotFound {
				return err
			}

			newCollect := &model.Collect{
				UserID: userID,
				PostID: postID,
			}
			if err := repoWithTx.CreateCollect(newCollect); err != nil {
				return err
			}

			collected = true
			return nil
		}

		if err := repoWithTx.DeleteCollect(collect); err != nil {
			return err
		}

		collected = false
		return nil
	})
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "toggle collect failed")
	}

	if collected {
		s.bumpPostHotScore(postID, postHotScoreCollect)
		return &InteractActionResult{Status: true, Message: "collect success"}, nil
	}

	s.bumpPostHotScore(postID, -postHotScoreCollect)
	return &InteractActionResult{Status: false, Message: "cancel collect success"}, nil
}

func (s *InteractService) Share(userID uint, postIDStr string) (*InteractActionResult, error) {
	postID, err := parsePostID(postIDStr)
	if err != nil {
		return nil, err
	}

	share := &model.Share{
		UserID:   userID,
		PostID:   postID,
		Platform: "internal",
		ShareURL: "",
	}

	if err := s.interactRepo.CreateShare(share); err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "share failed")
	}

	s.bumpPostHotScore(postID, postHotScoreShare)
	return &InteractActionResult{Message: "share success"}, nil
}

func (s *InteractService) GetInteractStatus(userID uint, postIDStr string) (map[string]interface{}, error) {
	postID, err := parsePostID(postIDStr)
	if err != nil {
		return nil, err
	}

	status, err := s.interactRepo.GetInteractStatus(userID, postID)
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "get interact status failed")
	}
	return status, nil
}

func (s *InteractService) GetInteractCount(postIDStr string) (map[string]int64, error) {
	postID, err := parsePostID(postIDStr)
	if err != nil {
		return nil, err
	}

	counts, err := s.interactRepo.GetInteractCount(postID)
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "get interact count failed")
	}
	return counts, nil
}

func (s *InteractService) bumpPostHotScore(postID uint, delta float64) {
	if s.cache == nil || postID == 0 || delta == 0 {
		return
	}
	_, _ = s.cache.ZIncrBy(s.ctx, postHotRankKey, delta, fmt.Sprintf("%d", postID))
}

func parsePostID(postIDStr string) (uint, error) {
	postID, err := strconv.ParseUint(postIDStr, 10, 32)
	if err != nil {
		return 0, core.NewBizError(http.StatusBadRequest, "invalid post id")
	}
	return uint(postID), nil
}
