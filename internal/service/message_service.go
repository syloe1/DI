package service

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"go-admin/internal/dao"
	"go-admin/internal/domain/model"
	"go-admin/pkg/core"
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

type MessageService struct {
	messageRepo dao.MessageRepository
	cache       dao.UserCache
	ctx         context.Context
}

func NewMessageService(messageRepo dao.MessageRepository, cache dao.UserCache, ctx context.Context) *MessageService {
	return &MessageService{
		messageRepo: messageRepo,
		cache:       cache,
		ctx:         ctx,
	}
}

func (s *MessageService) GetConversations(userID uint) ([]ConversationItem, error) {
	conversations, err := s.messageRepo.GetConversations(userID)
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "get conversations failed")
	}

	result := make([]ConversationItem, 0, len(conversations))
	for _, conv := range conversations {
		user, err := s.messageRepo.FindUserByID(conv.PeerUID)
		if err != nil {
			continue
		}

		unread, _ := s.messageRepo.CountUnreadMessages(userID, conv.PeerUID)
		result = append(result, ConversationItem{
			UID:      conv.PeerUID,
			Username: user.Username,
			Avatar:   "",
			LastMsg:  conv.LastMsg,
			LastTime: conv.LastTime,
			Unread:   unread,
		})
	}

	return result, nil
}

func (s *MessageService) GetMessageList(userID uint, peerID uint, page int, pageSize int) (*MessageListResult, error) {
	page, pageSize = normalizePagination(page, pageSize, 1, 20)
	offset := (page - 1) * pageSize

	peerUser, err := s.messageRepo.FindUserByID(peerID)
	if err != nil {
		return nil, core.NewBizError(http.StatusNotFound, "peer user not found")
	}

	messages, err := s.messageRepo.GetMessages(userID, peerID, offset, pageSize)
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "get messages failed")
	}

	if err := s.messageRepo.MarkMessagesAsRead(userID, peerID); err != nil {
		fmt.Printf("mark messages as read failed: %v\n", err)
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

func (s *MessageService) SendMessage(userID uint, toUID uint, content string) (*SendMessageResult, error) {
	toUser, err := s.messageRepo.FindUserByID(toUID)
	if err != nil {
		return nil, core.NewBizError(http.StatusNotFound, "target user not found")
	}

	if userID == toUID {
		return nil, core.NewBizError(http.StatusBadRequest, "cannot send message to yourself")
	}

	duplicate, err := s.checkDuplicateSubmit(userID, toUID, content)
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "duplicate submit check failed")
	}
	if duplicate {
		return nil, core.NewBizError(http.StatusTooManyRequests, "duplicate message submission")
	}

	message := &model.Message{
		FromUID: userID,
		ToUID:   toUID,
		Content: content,
		IsRead:  false,
	}

	if err := s.messageRepo.CreateMessage(message); err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "send message failed")
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

func (s *MessageService) DeleteMessage(userID uint, messageIDStr string) error {
	messageID, err := strconv.ParseUint(messageIDStr, 10, 32)
	if err != nil {
		return core.NewBizError(http.StatusBadRequest, "invalid message id")
	}

	message, err := s.messageRepo.FindMessageByID(uint(messageID))
	if err != nil {
		return core.NewBizError(http.StatusNotFound, "message not found")
	}

	if message.FromUID != userID {
		return core.NewBizError(http.StatusForbidden, "can only delete your own messages")
	}

	if err := s.messageRepo.DeleteMessage(message); err != nil {
		return core.NewBizError(http.StatusInternalServerError, "delete message failed")
	}

	return nil
}

func (s *MessageService) checkDuplicateSubmit(userID uint, toUID uint, content string) (bool, error) {
	key := fmt.Sprintf("duplicate:message:%d:%d:%s", userID, toUID, hashPayload(content))
	ok, err := s.cache.SetNX(s.ctx, key, "1", messageDuplicateSubmitTTL)
	if err != nil {
		return false, err
	}
	return !ok, nil
}
