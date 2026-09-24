package service

import (
	"go-admin/internal/domain/model"
	"go-admin/internal/dto"
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
	// 与 WS 路径一致：同一事务写消息 + outbox，relay 发布后由 consumer 实时广播给在线成员
	event := dto.GroupMessageCreatedEvent{
		Type:    "group_message_created",
		GroupID: groupID,
		FromUID: userID,
		Content: content,
	}
	if err := s.groupRepo.CreateGroupMessageWithOutboxBuilder(message, func(saved *model.ChatGroupMessage) (*model.MessageOutbox, error) {
		event.MessageID = saved.ID
		event.CreatedAt = saved.CreatedAt
		return buildGroupMessageOutbox(event)
	}); err != nil {
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
