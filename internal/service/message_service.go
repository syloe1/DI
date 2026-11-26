package service

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"di/internal/dao"
	"di/internal/domain/model"
	"di/internal/telemetry"
	"di/internal/transport"
)

const messageDuplicateSubmitTTL = 3 * time.Second

type ConversationItem struct {
	UID      uint   `json:"uid"`
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
	LastMsg  string `json:"last_msg"`
	LastTime string `json:"last_time"`
	Unread   int64  `json:"unread"`
}

type PeerUser struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
}

type MessageListItem struct {
	ID        uint      `json:"id"`
	FromUID   uint      `json:"from_uid"`
	ToUID     uint      `json:"to_uid"`
	Content   string    `json:"content"`
	IsRead    bool      `json:"is_read"`
	CreatedAt time.Time `json:"created_at"`
}

type MessageListResult struct {
	PeerUser PeerUser          `json:"peer_user"`
	Messages []MessageListItem `json:"messages"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
}

type SendMessageResult struct {
	MessageID uint      `json:"message_id"`
	ToUser    PeerUser  `json:"to_user"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// MessageSyncResult 断线补偿的增量同步结果
type MessageSyncResult struct {
	Messages []MessageListItem `json:"messages"`
	LastID   uint              `json:"last_id"`  // 下次 sync 的游标（客户端持久化）
	HasMore  bool              `json:"has_more"` // 是否还有更多，true 时用 LastID 继续拉
}

// MessageService 请求链路 Service：ctx 一律方法级传入（Context 反模式修复），不持有请求级上下文。
type MessageService struct {
	messageRepo dao.MessageRepository
	cache       dao.UserCache
	// onMessagesRead 已读回执回调（由 container 注入，避免 Service 循环依赖）：
	// 参数：senderUID=被读消息的发送方（要通知的人）、readerUID=发起已读的用户、messageIDs=被读消息 ID 列表。
	onMessagesRead func(senderUID uint, readerUID uint, messageIDs []uint)
	// onPrivateMessage 私信落库后的实时推送回调（由 container 注入）：
	// 参数 msg=刚落库的消息（含自增 ID 与 CreatedAt）。推给收件人在线连接。
	onPrivateMessage func(msg *model.Message)
}

func NewMessageService(messageRepo dao.MessageRepository, cache dao.UserCache, onMessagesRead func(uint, uint, []uint), onPrivateMessage func(*model.Message)) *MessageService {
	return &MessageService{
		messageRepo:      messageRepo,
		cache:            cache,
		onMessagesRead:   onMessagesRead,
		onPrivateMessage: onPrivateMessage,
	}
}

func (s *MessageService) GetConversations(ctx context.Context, userID uint, cursor uint, limit int) ([]ConversationItem, uint, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// 多取一条判断是否还有下一页（游标分页）
	conversations, err := s.messageRepo.GetConversations(ctx, userID, cursor, limit+1)
	if err != nil {
		return nil, 0, transport.NewBizError(http.StatusInternalServerError, "get conversations failed")
	}

	nextCursor := uint(0)
	if len(conversations) > limit {
		nextCursor = conversations[limit-1].LastID
		conversations = conversations[:limit]
	}
	if len(conversations) == 0 {
		return []ConversationItem{}, 0, nil
	}

	// N+1 修复：先收集全部 peerID，用两条批量查询替代循环内的 FindUserByID / CountUnreadMessages
	peerIDs := make([]uint, 0, len(conversations))
	for _, conv := range conversations {
		peerIDs = append(peerIDs, conv.PeerUID)
	}

	users, err := s.messageRepo.FindUsersByIDs(ctx, peerIDs)
	if err != nil {
		return nil, 0, transport.NewBizError(http.StatusInternalServerError, "load peer users failed")
	}
	unreadMap, err := s.messageRepo.CountUnreadByPeers(ctx, userID, peerIDs)
	if err != nil {
		return nil, 0, transport.NewBizError(http.StatusInternalServerError, "load unread counts failed")
	}

	result := make([]ConversationItem, 0, len(conversations))
	for _, conv := range conversations {
		user, ok := users[conv.PeerUID]
		if !ok {
			continue
		}
		result = append(result, ConversationItem{
			UID:      conv.PeerUID,
			Username: user.Username,
			Avatar:   "",
			LastMsg:  conv.LastMsg,
			LastTime: conv.LastTime,
			Unread:   unreadMap[conv.PeerUID],
		})
	}

	return result, nextCursor, nil
}

func (s *MessageService) GetMessageList(ctx context.Context, userID uint, peerID uint, page int, pageSize int) (*MessageListResult, error) {
	page, pageSize = normalizePagination(page, pageSize, 1, 20)
	offset := (page - 1) * pageSize

	peerUser, err := s.messageRepo.FindUserByID(ctx, peerID)
	if err != nil {
		return nil, transport.NewBizError(http.StatusNotFound, "peer user not found")
	}

	messages, err := s.messageRepo.GetMessages(ctx, userID, peerID, offset, pageSize)
	if err != nil {
		return nil, transport.NewBizError(http.StatusInternalServerError, "get messages failed")
	}

	// 标记已读 + 已读回执：先查出本次从未读→已读的消息 ID，再通知对方
	unreadIDs, err := s.messageRepo.FindUnreadMessageIDs(ctx, userID, peerID)
	if err != nil {
		return nil, transport.NewBizError(http.StatusInternalServerError, "query unread messages failed")
	}
	if len(unreadIDs) > 0 {
		if err := s.messageRepo.MarkMessagesAsRead(ctx, userID, peerID); err != nil {
			// 已读持久化失败：不发已读成功回执（避免对方误以为已读），记录结构化告警
			telemetry.Z().Errorf("mark messages as read failed user_id=%d peer_id=%d: %v", userID, peerID, err)
		} else if s.onMessagesRead != nil {
			s.onMessagesRead(peerID, userID, unreadIDs)
		}
	}

	result := make([]MessageListItem, 0, len(messages))
	for _, msg := range messages {
		result = append(result, MessageListItem{
			ID:        msg.ID,
			FromUID:   msg.FromUID,
			ToUID:     msg.ToUID,
			Content:   msg.Content,
			IsRead:    msg.IsRead,
			CreatedAt: msg.CreatedAt,
		})
	}

	return &MessageListResult{
		PeerUser: PeerUser{
			ID:       peerUser.ID,
			Username: peerUser.Username,
			Avatar:   "",
		},
		Messages: result,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *MessageService) SyncMessages(ctx context.Context, userID uint, afterID uint, limit int) (*MessageSyncResult, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	// 多取一条判断是否还有更多，避免额外 COUNT 查询
	messages, err := s.messageRepo.SyncMessages(ctx, userID, afterID, limit+1)
	if err != nil {
		return nil, transport.NewBizError(http.StatusInternalServerError, "sync messages failed")
	}
	hasMore := len(messages) > limit
	if hasMore {
		messages = messages[:limit]
	}

	result := make([]MessageListItem, 0, len(messages))
	lastID := afterID
	for _, msg := range messages {
		result = append(result, MessageListItem{
			ID:        msg.ID,
			FromUID:   msg.FromUID,
			ToUID:     msg.ToUID,
			Content:   msg.Content,
			IsRead:    msg.IsRead,
			CreatedAt: msg.CreatedAt,
		})
		if msg.ID > lastID {
			lastID = msg.ID
		}
	}

	return &MessageSyncResult{
		Messages: result,
		LastID:   lastID,
		HasMore:  hasMore,
	}, nil
}

func (s *MessageService) SendMessage(ctx context.Context, userID uint, toUID uint, content string) (*SendMessageResult, error) {
	toUser, err := s.messageRepo.FindUserByID(ctx, toUID)
	if err != nil {
		return nil, transport.NewBizError(http.StatusNotFound, "target user not found")
	}

	if userID == toUID {
		return nil, transport.NewBizError(http.StatusBadRequest, "cannot send message to yourself")
	}

	duplicate, err := s.checkDuplicateSubmit(ctx, userID, toUID, content)
	if err != nil {
		return nil, transport.NewBizError(http.StatusInternalServerError, "duplicate submit check failed")
	}
	if duplicate {
		return nil, transport.NewBizError(http.StatusTooManyRequests, "duplicate message submission")
	}

	message := &model.Message{
		FromUID: userID,
		ToUID:   toUID,
		Content: content,
		IsRead:  false,
	}

	if err := s.messageRepo.CreateMessage(ctx, message); err != nil {
		return nil, transport.NewBizError(http.StatusInternalServerError, "send message failed")
	}

	// 实时推送：收件人在线则立即收到 WS message 事件（写通道有缓冲 + 慢客户端丢弃，不阻塞请求）。
	// 离线则静默，下次打开会话时由 list/sync 兜底。
	if s.onPrivateMessage != nil {
		s.onPrivateMessage(message)
	}

	return &SendMessageResult{
		MessageID: message.ID,
		ToUser: PeerUser{
			ID:       toUser.ID,
			Username: toUser.Username,
			Avatar:   "",
		},
		Content:   content,
		CreatedAt: message.CreatedAt,
	}, nil
}

func (s *MessageService) DeleteMessage(ctx context.Context, userID uint, messageIDStr string) error {
	messageID, err := strconv.ParseUint(messageIDStr, 10, 32)
	if err != nil {
		return transport.NewBizError(http.StatusBadRequest, "invalid message id")
	}

	message, err := s.messageRepo.FindMessageByID(ctx, uint(messageID))
	if err != nil {
		return transport.NewBizError(http.StatusNotFound, "message not found")
	}

	if message.FromUID != userID {
		return transport.NewBizError(http.StatusForbidden, "can only delete your own messages")
	}

	if err := s.messageRepo.DeleteMessage(ctx, message); err != nil {
		return transport.NewBizError(http.StatusInternalServerError, "delete message failed")
	}

	return nil
}

func (s *MessageService) checkDuplicateSubmit(ctx context.Context, userID uint, toUID uint, content string) (bool, error) {
	key := fmt.Sprintf("duplicate:message:%d:%d:%s", userID, toUID, hashPayload(content))
	ok, err := s.cache.SetNX(ctx, key, "1", messageDuplicateSubmitTTL)
	if err != nil {
		return false, err
	}
	return !ok, nil
}
