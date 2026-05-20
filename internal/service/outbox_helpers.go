package service

import (
	"encoding/json"
	"strconv"
	"time"

	"go-admin/internal/domain/model"
	"go-admin/internal/dto"
)

func buildGroupMessageOutbox(event dto.GroupMessageCreatedEvent) (*model.MessageOutbox, error) {
	body, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}
	return &model.MessageOutbox{
		EventType:   event.Type,
		AggregateID: strconv.FormatUint(uint64(event.MessageID), 10),
		Payload:     string(body),
		Status:      model.MessageOutboxStatusPending,
		NextRetryAt: time.Now(),
	}, nil
}
