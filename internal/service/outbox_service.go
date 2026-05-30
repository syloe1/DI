package service

import (
	"context"
	"log"
	"time"

	"go-admin/internal/dao"
	"go-admin/internal/domain/model"
	"go-admin/pkg/core"
)

type OutboxPublisher interface {
	PublishRaw(ctx context.Context, body []byte) error
}

type OutboxService struct {
	repo      dao.OutboxRepository
	publisher OutboxPublisher
	claimer   string  //抢占标识
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
	s.refreshMetrics()
	items, err := s.repo.ClaimDue(s.batchSize, now, s.claimer, s.claimTTL)
	if err != nil {
		log.Printf("list outbox failed: %v", err)
		return
	}

	for _, item := range items {
		if err := s.publisher.PublishRaw(ctx, []byte(item.Payload)); err != nil {
			retryCount := item.RetryCount + 1
			nextRetryAt := time.Now().Add(backoffDuration(retryCount))
			_ = s.repo.MarkFailed(item.ID, retryCount, nextRetryAt, err.Error())
			core.Metrics.OutboxPublishFailed.Add(1)
			continue
		}
		//db标记为已投递， 
		_ = s.repo.MarkPublished(item.ID, time.Now())
		core.Metrics.OutboxPublished.Add(1)
	}
	s.refreshMetrics()
}

func (s *OutboxService) refreshMetrics() {
	pending, err := s.repo.CountByStatus(model.MessageOutboxStatusPending)
	if err == nil {
		core.Metrics.OutboxPending.Store(pending)
	}

	processing, err := s.repo.CountByStatus(model.MessageOutboxStatusProcessing)
	if err == nil {
		core.Metrics.OutboxProcessing.Store(processing)
	}

	failed, err := s.repo.CountByStatus(model.MessageOutboxStatusFailed)
	if err == nil {
		core.Metrics.OutboxFailed.Store(failed)
	}
}
//规避重试风暴 指数时间上升
func backoffDuration(retryCount int) time.Duration {
	if retryCount <= 0 {
		return time.Second
	}
	if retryCount > 6 {
		retryCount = 6
	}
	return time.Duration(1<<retryCount) * time.Second
}
