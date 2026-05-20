package service

import (
	"context"
	"log"
	"time"

	"go-admin/internal/dao"
	"go-admin/pkg/core"
)

type OutboxPublisher interface {
	PublishRaw(ctx context.Context, body []byte) error
}

type OutboxService struct {
	repo      dao.OutboxRepository
	publisher OutboxPublisher
	interval  time.Duration
	batchSize int
}

func NewOutboxService(repo dao.OutboxRepository, publisher OutboxPublisher) *OutboxService {
	return &OutboxService{
		repo:      repo,
		publisher: publisher,
		interval:  2 * time.Second,
		batchSize: 100,
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
	items, err := s.repo.ListDue(s.batchSize, now)
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
		_ = s.repo.MarkPublished(item.ID, time.Now())
		core.Metrics.OutboxPublished.Add(1)
	}
}

func backoffDuration(retryCount int) time.Duration {
	if retryCount <= 0 {
		return time.Second
	}
	if retryCount > 6 {
		retryCount = 6
	}
	return time.Duration(1<<retryCount) * time.Second
}
