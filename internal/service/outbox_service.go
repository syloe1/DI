package service

import (
	"context"
	"encoding/json"
	"time"

	"di/internal/dao"
	"di/internal/domain/model"
	"di/internal/dto"
	"di/internal/telemetry"
)

// outbox后台扫描投递服务
type OutboxPublisher interface {
	PublishRaw(ctx context.Context, body []byte) error
}

type OutboxService struct {
	repo      dao.OutboxRepository
	publisher OutboxPublisher
	claimer   string //抢占标识
	interval  time.Duration
	batchSize int
	claimTTL  time.Duration
}

func NewOutboxService(repo dao.OutboxRepository, publisher OutboxPublisher, claimer string) *OutboxService {
	return &OutboxService{
		repo:      repo,
		publisher: publisher,
		claimer:   claimer,
		interval:  2 * time.Second,
		batchSize: 100,
		claimTTL:  30 * time.Second,
	}
}

func (s *OutboxService) Start(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.publishDue(ctx)
			}
		}
	}()
}

func (s *OutboxService) publishDue(ctx context.Context) {
	now := time.Now()
	s.refreshMetrics(ctx)
	items, err := s.repo.ClaimDue(ctx, s.batchSize, now,
		s.claimer, s.claimTTL)
	if err != nil {
		telemetry.Z().Errorf("list outbox failed: %v", err)
		return
	}

	for _, item := range items {
		if err := s.publisher.PublishRaw(ctx, []byte(item.Payload)); err != nil {
			s.logPublishError(item, "publish outbox message failed", err)
			retryCount := item.RetryCount + 1
			nextRetryAt := time.Now().Add(backoffDuration(retryCount))
			if markErr := s.repo.MarkFailed(ctx, item.ID, retryCount, nextRetryAt, err.Error()); markErr != nil {
				s.logPublishError(item, "mark outbox failed", markErr)
			}
			telemetry.Metrics.OutboxPublishFailed.Add(1)
			continue
		}
		//db标记为已投递，
		if markErr := s.repo.MarkPublished(ctx, item.ID, time.Now()); markErr != nil {
			s.logPublishError(item, "mark outbox published", markErr)
		}
		telemetry.Metrics.OutboxPublished.Add(1)
	}
	s.refreshMetrics(ctx)
}

func (s *OutboxService) refreshMetrics(ctx context.Context) {
	pending, err := s.repo.CountByStatus(ctx, model.MessageOutboxStatusPending)
	if err == nil {
		telemetry.Metrics.OutboxPending.Store(pending)
	}

	processing, err := s.repo.CountByStatus(ctx, model.MessageOutboxStatusProcessing)
	if err == nil {
		telemetry.Metrics.OutboxProcessing.Store(processing)
	}

	failed, err := s.repo.CountByStatus(ctx, model.MessageOutboxStatusFailed)
	if err == nil {
		telemetry.Metrics.OutboxFailed.Store(failed)
	}
}

// logPublishError 发布/标记出错日志带关键字段（从 outbox payload 反解事件）。
// message_id/group_id/user_id/request_id 缺失时为对应零值/空，便于线上定位到具体群消息。
func (s *OutboxService) logPublishError(item model.MessageOutbox, msg string, err error) {
	var event dto.GroupMessageCreatedEvent
	_ = json.Unmarshal([]byte(item.Payload), &event)
	telemetry.Z().With(
		"outbox_id", item.ID,
		"message_id", event.MessageID,
		"group_id", event.GroupID,
		"user_id", event.FromUID,
		"request_id", event.RequestID,
	).Errorf("%s: %v", msg, err)
}

// 规避重试风暴 指数时间上升
func backoffDuration(retryCount int) time.Duration {
	if retryCount <= 0 {
		return time.Second
	}
	if retryCount > 6 {
		retryCount = 6
	}
	return time.Duration(1<<retryCount) * time.Second
}
