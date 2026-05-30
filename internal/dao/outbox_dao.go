package dao

import (
	"time"

	"go-admin/internal/domain/model"

	"gorm.io/gorm"
)

type OutboxRepository interface {
	ClaimDue(limit int, now time.Time, claimer string, staleAfter time.Duration) ([]model.MessageOutbox, error)
	CountByStatus(status string) (int64, error)
	MarkPublished(id uint, now time.Time) error
	MarkFailed(id uint, retryCount int, nextRetryAt time.Time, lastError string) error
}

type GormOutboxRepository struct {
	db *gorm.DB
}

func NewGormOutboxRepository(db *gorm.DB) *GormOutboxRepository {
	return &GormOutboxRepository{db: db}
}

func (r *GormOutboxRepository) ClaimDue(limit int, now time.Time, claimer string, staleAfter time.Duration) ([]model.MessageOutbox, error) {
	var outboxes []model.MessageOutbox
	staleBefore := now.Add(-staleAfter)

	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Raw(
			`SELECT * FROM message_outboxes
			 WHERE (
				(status = ? AND next_retry_at <= ?)
				OR (status = ? AND claimed_at IS NOT NULL AND claimed_at <= ?)
			 )
			 ORDER BY next_retry_at ASC, id ASC
			 LIMIT ?
			 FOR UPDATE SKIP LOCKED`,
			model.MessageOutboxStatusPending,
			now,
			model.MessageOutboxStatusProcessing,
			staleBefore,
			limit,
		).Scan(&outboxes).Error; err != nil {
			return err
		}

		if len(outboxes) == 0 {
			return nil
		}

		ids := make([]uint, 0, len(outboxes))
		claimedAt := now
		for i := range outboxes {
			ids = append(ids, outboxes[i].ID)
		}

		if err := tx.Model(&model.MessageOutbox{}).
			Where("id IN ?", ids).
			Updates(map[string]interface{}{
				"status":     model.MessageOutboxStatusProcessing,
				"claimed_at": &claimedAt,
				"claimed_by": claimer,
				"last_error": "",
			}).Error; err != nil {
			return err
		}

		for i := range outboxes {
			outboxes[i].Status = model.MessageOutboxStatusProcessing
			outboxes[i].ClaimedAt = &claimedAt
			outboxes[i].ClaimedBy = claimer
			outboxes[i].LastError = ""
		}

		return nil
	})

	return outboxes, err
}

func (r *GormOutboxRepository) CountByStatus(status string) (int64, error) {
	var count int64
	//SELECT COUNT(*) FROM message_outboxes WHERE status = ?
	err := r.db.Model(&model.MessageOutbox{}).Where("status = ?", status).Count(&count).Error
	return count, err
}

func (r *GormOutboxRepository) MarkPublished(id uint, now time.Time) error {
	/*
		update message_ouotboxes
		set status = published, published_at = ? where id = ?
	*/
	return r.db.Model(&model.MessageOutbox{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":       model.MessageOutboxStatusPublished,
			"published_at": &now,
			"claimed_at":   nil,
			"claimed_by":   "",
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
			"claimed_at":    nil,
			"claimed_by":    "",
			"last_error":    lastError,
		}).Error
}
