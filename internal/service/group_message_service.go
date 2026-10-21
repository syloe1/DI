package service

import (
	"go-admin/internal/domain/model"
	"go-admin/internal/dto"
	"go-admin/pkg/core"
	"net/http"
	"strings"
)

// -------------------------- 群消息：发送 / 分页查询 --------------------------

func (s *GroupService) SendGroupMessage(userID, groupID uint, content string) (*GroupMessageItem, error) {
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

	// 发送者资料：响应/实时事件都直接带 username，刷新前后显示一致
	var username string
	if user, err := s.groupRepo.FindUserByID(userID); err == nil && user != nil {
		username = user.Username
	}

	message := &model.ChatGroupMessage{
		GroupID:   groupID,
		SenderUID: userID,
		Content:   content,
	}
	// 与 WS 路径一致：同一事务写消息 + outbox，relay 发布后由 consumer 实时广播给在线成员
	event := dto.GroupMessageCreatedEvent{
		Type:     "group_message_created",
		GroupID:  groupID,
		FromUID:  userID,
		Username: username,
		Content:  content,
	}
	if err := s.groupRepo.CreateGroupMessageWithOutboxBuilder(message, func(saved *model.ChatGroupMessage) (*model.MessageOutbox, error) {
		event.MessageID = saved.ID
		event.CreatedAt = saved.CreatedAt
		return buildGroupMessageOutbox(event)
	}); err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "send group message failed")
	}

	return &GroupMessageItem{
		ID:        message.ID,
		GroupID:   message.GroupID,
		SenderUID: message.SenderUID,
		Username:  username,
		Content:   message.Content,
		CreatedAt: message.CreatedAt,
	}, nil
}

func (s *GroupService) GetGroupMessages(userID, groupID uint, page, pageSize int) ([]GroupMessageItem, error) {
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

	// 批量补发送者 username（一次 IN 查询，前端不用再逐个请求 /user/{id}）
	senderIDs := make([]uint, 0, len(messages))
	seen := make(map[uint]struct{}, len(messages))
	for _, m := range messages {
		if _, ok := seen[m.SenderUID]; ok {
			continue
		}
		seen[m.SenderUID] = struct{}{}
		senderIDs = append(senderIDs, m.SenderUID)
	}
	userMap, err := s.groupRepo.FindUsersByIDs(senderIDs)
	if err != nil {
		userMap = map[uint]*model.User{}
	}

	result := make([]GroupMessageItem, 0, len(messages))
	for _, m := range messages {
		username := ""
		if user, ok := userMap[m.SenderUID]; ok {
			username = user.Username
		}
		result = append(result, GroupMessageItem{
			ID:        m.ID,
			GroupID:   m.GroupID,
			SenderUID: m.SenderUID,
			Username:  username,
			Content:   m.Content,
			CreatedAt: m.CreatedAt,
		})
	}

	return result, nil
}
