package service

import (
	"context"
	"di/internal/domain/model"
	"di/internal/dto"
	"di/internal/telemetry"
	"di/internal/transport"
	"net/http"
	"strings"
)

// -------------------------- 群消息：发送 / 分页查询 --------------------------

func (s *GroupService) SendGroupMessage(ctx context.Context, userID, groupID uint, content string, requestID string) (*GroupMessageItem, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, transport.NewBizError(http.StatusBadRequest, "content is required")
	}
	if len(content) > 2000 {
		return nil, transport.NewBizError(http.StatusBadRequest, "content is too long")
	}

	if _, err := s.getNormalGroup(ctx, groupID); err != nil {
		return nil, err
	}

	if _, err := s.getActiveMember(ctx, groupID, userID, "not a group member"); err != nil {
		return nil, err
	}

	// 发送者资料：响应/实时事件都直接带 username，刷新前后显示一致
	var username string
	if user, err := s.groupRepo.FindUserByID(ctx, userID); err == nil && user != nil {
		username = user.Username
	}

	message := &model.ChatGroupMessage{
		GroupID:   groupID,
		SenderUID: userID,
		Content:   content,
	}
	// 与 WS 路径一致：同一事务写消息 + outbox，relay 发布后由 consumer 实时广播给在线成员
	event := dto.GroupMessageCreatedEvent{
		Type:      "group_message_created",
		GroupID:   groupID,
		FromUID:   userID,
		Username:  username,
		Content:   content,
		RequestID: requestID, // HTTP 请求透传 request_id 到事件/日志；WS 路径留空
	}
	if err := s.groupRepo.CreateGroupMessageWithOutboxBuilder(ctx, message, func(saved *model.ChatGroupMessage) (*model.MessageOutbox, error) {
		event.MessageID = saved.ID
		event.CreatedAt = saved.CreatedAt
		return buildGroupMessageOutbox(event)
	}); err != nil {
		return nil, transport.NewBizError(http.StatusInternalServerError, "send group message failed")
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

func (s *GroupService) GetGroupMessages(ctx context.Context, userID, groupID uint, page, pageSize int) ([]GroupMessageItem, error) {
	page, pageSize = normalizePagination(page, pageSize, 1, 20)
	offset := (page - 1) * pageSize

	if _, err := s.getNormalGroup(ctx, groupID); err != nil {
		return nil, err
	}

	if _, err := s.getActiveMember(ctx, groupID, userID, "not a group member"); err != nil {
		return nil, err
	}

	messages, err := s.groupRepo.ListGroupMessages(ctx, groupID, offset, pageSize)
	if err != nil {
		return nil, transport.NewBizError(http.StatusInternalServerError, "get group messages failed")
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
	userMap, err := s.groupRepo.FindUsersByIDs(ctx, senderIDs)
	if err != nil {
		telemetry.Z().Warnf("load sender usernames failed group_id=%d: %v", groupID, err)
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

// GroupMessageSyncResult 群消息断线补偿的增量同步结果
type GroupMessageSyncResult struct {
	Messages []GroupMessageItem `json:"messages"`
	LastID   uint               `json:"last_id"`  // 下次 sync 的游标（客户端持久化）
	HasMore  bool               `json:"has_more"` // 是否还有更多，true 时用 LastID 继续拉
}

// SyncGroupMessages 群消息增量同步：返回 groupID 内 id 大于 afterID 的消息，按 id 升序。
// 语义与私聊 /auth/message/sync 一致；要求调用者是该群 active 成员，群不存在/已解散沿用 404/410。
func (s *GroupService) SyncGroupMessages(ctx context.Context, userID, groupID uint, afterID uint, limit int) (*GroupMessageSyncResult, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	if _, err := s.getNormalGroup(ctx, groupID); err != nil {
		return nil, err
	}
	if _, err := s.getActiveMember(ctx, groupID, userID, "not a group member"); err != nil {
		return nil, err
	}

	// 多取一条判断是否还有更多，避免额外 COUNT 查询
	messages, err := s.groupRepo.SyncGroupMessages(ctx, groupID, afterID, limit+1)
	if err != nil {
		return nil, transport.NewBizError(http.StatusInternalServerError, "sync group messages failed")
	}
	hasMore := len(messages) > limit
	if hasMore {
		messages = messages[:limit]
	}

	// 批量补发送者 username（与群消息列表一致，前端不用再逐个 /user/{id}）
	senderIDs := make([]uint, 0, len(messages))
	seen := make(map[uint]struct{}, len(messages))
	for _, m := range messages {
		if _, ok := seen[m.SenderUID]; ok {
			continue
		}
		seen[m.SenderUID] = struct{}{}
		senderIDs = append(senderIDs, m.SenderUID)
	}
	userMap, err := s.groupRepo.FindUsersByIDs(ctx, senderIDs)
	if err != nil {
		telemetry.Z().Warnf("load sender usernames failed group_id=%d: %v", groupID, err)
		userMap = map[uint]*model.User{}
	}

	result := make([]GroupMessageItem, 0, len(messages))
	lastID := afterID
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
		if m.ID > lastID {
			lastID = m.ID
		}
	}

	return &GroupMessageSyncResult{
		Messages: result,
		LastID:   lastID,
		HasMore:  hasMore,
	}, nil
}
