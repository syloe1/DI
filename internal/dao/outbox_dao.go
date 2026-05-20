package dao

import (
	"time"

	"go-admin/internal/domain/model"

	"gorm.io/gorm"
)

type OutboxRepository interface {
	ListDue(limit int, now time.Time) ([]model.MessageOutbox, error)
	MarkPublished(id uint, now time.Time) error
	MarkFailed(id uint, retryCount int, nextRetryAt time.Time, lastError string) error
}

type GormOutboxRepository struct {
	db *gorm.DB
}

func NewGormOutboxRepository(db *gorm.DB) *GormOutboxRepository {
	return &GormOutboxRepository{db: db}
}

func (r *GormOutboxRepository) ListDue(limit int, now time.Time) ([]model.MessageOutbox, error) {
	var outboxes []model.MessageOutbox
	err := r.db.Where("status = ? AND next_retry_at <= ?", model.MessageOutboxStatusPending, now).
		Order("next_retry_at ASC, id ASC").
		Limit(limit).
		Find(&outboxes).Error
	return outboxes, err
}

func (r *GormOutboxRepository) MarkPublished(id uint, now time.Time) error {
	return r.db.Model(&model.MessageOutbox{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":       model.MessageOutboxStatusPublished,
			"published_at": &now,
			"last_error":   "",
		}).Error
}

func (r *GormOutboxRepository) MarkFailed(id uint, retryCount int, nextRetryAt time.Time, lastError string) error {
	status := model.MessageOutboxStatusPending
	if retryCount >= 10 {
		status = model.MessageOutboxStatusFailed
	}

	return r.db.Model(&model.MessageOutbox{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":        status,
			"retry_count":   retryCount,
			"next_retry_at": nextRetryAt,
			"last_error":    lastError,
		}).Error
}
