package service

import (
	"go-admin/internal/domain/model"
	"go-admin/pkg/core"
	"net/http"
	"strings"
)

// -------------------------- 群消息：发送 / 分页查询 --------------------------

func (s *GroupService) SendGroupMessage(userID, groupID uint, content string) (*model.ChatGroupMessage, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, core.NewBizError(http.StatusBadRequest, "content is required")
	}
	if len(content) > 2000 {
		return nil, core.NewBizError(http.StatusBadRequest, "content is too long")
	}

	if _, err := s.getNormalGroup(groupID); err != nil {
		return nil, err
	}

	if _, err := s.getActiveMember(groupID, userID, "not a group member"); err != nil {
		return nil, err
	}

	message := &model.ChatGroupMessage{
		GroupID:   groupID,
		SenderUID: userID,
		Content:   content,
	}

	if err := s.groupRepo.CreateGroupMessage(message); err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "send group message failed")
	}

	return message, nil
}

func (s *GroupService) GetGroupMessages(userID, groupID uint, page, pageSize int) ([]model.ChatGroupMessage, error) {
	page, pageSize = normalizePagination(page, pageSize, 1, 20)
	offset := (page - 1) * pageSize

	if _, err := s.getNormalGroup(groupID); err != nil {
		return nil, err
	}

	if _, err := s.getActiveMember(groupID, userID, "not a group member"); err != nil {
		return nil, err
	}

	messages, err := s.groupRepo.ListGroupMessages(groupID, offset, pageSize)
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "get group messages failed")
	}

	return messages, nil
}
