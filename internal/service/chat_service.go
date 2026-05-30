package service

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go-admin/internal/dao"
	"go-admin/internal/domain/model"
	"go-admin/internal/dto"
	"go-admin/pkg/core"
)

type MessagePublisher interface {
	Publish(ctx context.Context, exchange string, routingKey string, body []byte) error
}

type GroupMessageReq struct {
	RequestID string `json:"request_id"`
	GroupID   string `json:"group_id"`
	UID       string `json:"uid"`
	Content   string `json:"content"`
}

type GroupMessageACK struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	MessageID string `json:"message_id"`
	Status    string `json:"status"`
}

type ChatService struct {
	groupRepo dao.GroupRepository
	publisher MessagePublisher
	exchange  string
}

func NewChatService(groupRepo dao.GroupRepository, publisher MessagePublisher, exchange string) *ChatService {
	return &ChatService{
		groupRepo: groupRepo,
		publisher: publisher,
		exchange:  exchange,
	}
}

func (s *ChatService) HandleGroupMessage(ctx context.Context, req GroupMessageReq) (*GroupMessageACK, error) {
	groupID64, err := strconv.ParseUint(req.GroupID, 10, 32)
	if err != nil || groupID64 == 0 {
		return nil, errors.New("invalid group_id")
	}

	uid64, err := strconv.ParseUint(req.UID, 10, 32)
	if err != nil || uid64 == 0 {
		return nil, errors.New("invalid uid")
	}

	groupID := uint(groupID64)
	uid := uint(uid64)
	content := strings.TrimSpace(req.Content)
	if content == "" {
		return nil, errors.New("invalid content")
	}

	if _, err := s.getNormalGroup(groupID); err != nil {
		return nil, err
	}
	if _, err := s.getActiveMember(groupID, uid); err != nil {
		return nil, err
	}

	message := &model.ChatGroupMessage{
		GroupID:   groupID,
		SenderUID: uid,
		Content:   content,
	}
	//MQ消息体
	event := dto.GroupMessageCreatedEvent{
		Type:      "group_message_created",
		RequestID: req.RequestID,
		GroupID:   groupID,
		FromUID:   uid,
		Content:   content,
		CreatedAt: time.Now(),
	}
	if err := s.groupRepo.CreateGroupMessageWithOutboxBuilder(message, func(saved *model.ChatGroupMessage) (*model.MessageOutbox, error) {
		event.MessageID = saved.ID
		event.CreatedAt = saved.CreatedAt
		return buildGroupMessageOutbox(event)
	}); err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "save group message failed")
	}

	return &GroupMessageACK{
		Type:      "group_message_ack",
		RequestID: req.RequestID,
		MessageID: strconv.FormatUint(uint64(message.ID), 10),
		Status:    "stored",
	}, nil
}

func (s *ChatService) getNormalGroup(groupID uint) (*model.ChatGroup, error) {
	group, err := s.groupRepo.FindGroupByID(groupID)
	if err != nil {
		return nil, core.NewBizError(http.StatusNotFound, "group not found")
	}
	if group.Status != model.ChatGroupStatusNormal {
		return nil, core.NewBizError(http.StatusBadRequest, "group is not available")
	}
	return group, nil
}

func (s *ChatService) getActiveMember(groupID, userID uint) (*model.ChatGroupMember, error) {
	member, err := s.groupRepo.FindMember(groupID, userID)
	if err != nil {
		return nil, core.NewBizError(http.StatusForbidden, "not a group member")
	}
	if member.Status != model.ChatGroupMemberStatusActive {
		return nil, core.NewBizError(http.StatusForbidden, "not a group member")
	}
	return member, nil
}
