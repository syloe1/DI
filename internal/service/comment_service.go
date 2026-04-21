package service

import (
	"net/http"
	"strconv"

	"go-admin/internal/dao"
	"go-admin/internal/domain/model"
	"go-admin/pkg/core"

	"gorm.io/gorm"
)

type CommentService struct {
	commentRepo dao.CommentRepository
}

func NewCommentService(commentRepo dao.CommentRepository) *CommentService {
	return &CommentService{commentRepo: commentRepo}
}

func (s *CommentService) CreateComment(postID uint, userID uint, username string, content string, parentID uint) (*model.Comment, error) {
	if _, err := s.commentRepo.FindPostByID(postID); err != nil {
		return nil, core.NewBizError(http.StatusNotFound, "post not found")
	}

	comment := model.Comment{
		PostID:   postID,
		UserID:   userID,
		Username: username,
		Content:  content,
		ParentId: parentID,
		Status:   model.CommentStatusNormal,
	}

	if err := s.commentRepo.Create(&comment); err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "create comment failed")
	}

	return &comment, nil
}

func (s *CommentService) DeleteComment(commentID string, currentUserID uint, currentUserRole string) error {
	comment, err := s.commentRepo.FindByID(commentID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return core.NewBizError(http.StatusNotFound, "comment not found")
		}
		return core.NewBizError(http.StatusInternalServerError, "query comment failed")
	}

	if comment.UserID != currentUserID {
		if currentUserRole == model.RoleSuperAdmin {
		} else if currentUserRole == model.RoleAdmin {
			commentUser, err := s.commentRepo.FindUserByID(comment.UserID)
			if err == nil && commentUser.Role != model.RoleUser {
				return core.NewBizError(http.StatusForbidden, "admin can only delete normal user comments")
			}
		} else {
			return core.NewBizError(http.StatusForbidden, "no permission to delete this comment")
		}
	}

	if err := s.commentRepo.Delete(comment); err != nil {
		return core.NewBizError(http.StatusInternalServerError, "delete comment failed")
	}

	return nil
}

func (s *CommentService) UpdateComment(commentID string, currentUserID uint, content string) error {
	comment, err := s.commentRepo.FindByID(commentID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return core.NewBizError(http.StatusNotFound, "comment not found")
		}
		return core.NewBizError(http.StatusInternalServerError, "query comment failed")
	}

	if comment.UserID != currentUserID {
		return core.NewBizError(http.StatusForbidden, "no permission to update this comment")
	}

	updates := map[string]interface{}{
		"content": content,
	}
	if err := s.commentRepo.Update(comment, updates); err != nil {
		return core.NewBizError(http.StatusInternalServerError, "update comment failed")
	}

	return nil
}

func (s *CommentService) GetPostComments(postID string) ([]model.Comment, error) {
	comments, err := s.commentRepo.FindByPostID(postID)
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "get post comments failed")
	}
	return comments, nil
}

func (s *CommentService) GetUserComments(userID uint) ([]model.Comment, error) {
	comments, err := s.commentRepo.FindByUserID(userID)
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "get user comments failed")
	}
	return comments, nil
}

func (s *CommentService) GetPublicUserComments(userIDStr string) ([]model.Comment, error) {
	userID, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil {
		return nil, core.NewBizError(http.StatusBadRequest, "invalid user id")
	}

	comments, err := s.commentRepo.FindByUserID(uint(userID))
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "get user comments failed")
	}
	return comments, nil
}
