package service

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"di/internal/domain/model"
	"di/internal/telemetry"
	"gorm.io/gorm"
)

// ==================== fakes ====================

// fakeOutboxRepo 内存实现 dao.OutboxRepository，记录状态流转调用。
type fakeOutboxRepo struct {
	claimResult  []model.MessageOutbox
	claimErr     error
	publishedIDs []uint
	failedCalls  []failedCall
	counts       map[string]int64
}

type failedCall struct {
	id          uint
	retryCount  int
	nextRetryAt time.Time
	lastError   string
}

func (f *fakeOutboxRepo) ClaimDue(ctx context.Context, limit int, now time.Time, claimer string, staleAfter time.Duration) ([]model.MessageOutbox, error) {
	return f.claimResult, f.claimErr
}

func (f *fakeOutboxRepo) CountByStatus(ctx context.Context, status string) (int64, error) {
	return f.counts[status], nil
}

func (f *fakeOutboxRepo) MarkPublished(ctx context.Context, id uint, now time.Time) error {
	f.publishedIDs = append(f.publishedIDs, id)
	return nil
}

func (f *fakeOutboxRepo) MarkFailed(ctx context.Context, id uint, retryCount int, nextRetryAt time.Time, lastError string) error {
	f.failedCalls = append(f.failedCalls, failedCall{id: id, retryCount: retryCount, nextRetryAt: nextRetryAt, lastError: lastError})
	return nil
}

// fakePublisher 内存实现 OutboxPublisher；fail 时模拟 MQ 不可用。
type fakePublisher struct {
	failPayloads map[string]bool // key=payload，命中则模拟发布失败
	published    []string
}

func newFakePublisher() *fakePublisher {
	return &fakePublisher{failPayloads: map[string]bool{}}
}

func (p *fakePublisher) PublishRaw(ctx context.Context, body []byte) error {
	payload := string(body)
	if p.failPayloads[payload] {
		return errors.New("mq unavailable")
	}
	p.published = append(p.published, payload)
	return nil
}

func newOutboxForTest(repo *fakeOutboxRepo, publisher *fakePublisher) *OutboxService {
	return &OutboxService{
		repo:      repo,
		publisher: publisher,
		claimer:   "test-instance",
		interval:  time.Second,
		batchSize: 100,
		claimTTL:  30 * time.Second,
	}
}

func outboxItem(id uint, payload string, retryCount int) model.MessageOutbox {
	return model.MessageOutbox{
		Model:      gorm.Model{ID: id},
		Payload:    payload,
		Status:     model.MessageOutboxStatusProcessing,
		RetryCount: retryCount,
	}
}

// ==================== tests ====================

func TestOutboxPublishDue_Success(t *testing.T) {
	repo := &fakeOutboxRepo{claimResult: []model.MessageOutbox{outboxItem(1, `{"event":"msg"}`, 0)}}
	pub := newFakePublisher()
	svc := newOutboxForTest(repo, pub)

	svc.publishDue(context.Background())

	if len(repo.publishedIDs) != 1 || repo.publishedIDs[0] != 1 {
		t.Errorf("MarkPublished ids = %v, want [1]", repo.publishedIDs)
	}
	if len(repo.failedCalls) != 0 {
		t.Errorf("unexpected MarkFailed calls: %+v", repo.failedCalls)
	}
	if len(pub.published) != 1 || pub.published[0] != `{"event":"msg"}` {
		t.Errorf("publisher got %v, want 1 success", pub.published)
	}
}

func TestOutboxPublishDue_PublishFailed_MarksRetry(t *testing.T) {
	repo := &fakeOutboxRepo{claimResult: []model.MessageOutbox{outboxItem(7, `{"event":"fail"}`, 0)}}
	pub := newFakePublisher()
	pub.failPayloads[`{"event":"fail"}`] = true
	svc := newOutboxForTest(repo, pub)

	before := telemetry.Metrics.OutboxPublishFailed.Load()
	svc.publishDue(context.Background())
	after := telemetry.Metrics.OutboxPublishFailed.Load()

	if len(repo.publishedIDs) != 0 {
		t.Errorf("MarkPublished called %v, want none", repo.publishedIDs)
	}
	if len(repo.failedCalls) != 1 {
		t.Fatalf("MarkFailed calls = %d, want 1", len(repo.failedCalls))
	}
	fc := repo.failedCalls[0]
	if fc.id != 7 {
		t.Errorf("failed id = %d, want 7", fc.id)
	}
	if fc.retryCount != 1 {
		t.Errorf("retryCount = %d, want 1（首次失败 +1）", fc.retryCount)
	}
	// 首次失败 → backoffDuration(1)=2s，下次重试应在 now+2s 附近
	expected := time.Now().Add(backoffDuration(1))
	if d := fc.nextRetryAt.Sub(expected); d > 100*time.Millisecond || d < -100*time.Millisecond {
		t.Errorf("nextRetryAt = %v, want ~%v", fc.nextRetryAt, expected)
	}
	if after-before != 1 {
		t.Errorf("OutboxPublishFailed delta = %d, want 1", after-before)
	}
}

func TestOutboxPublishDue_Mixed_SuccessAndFail(t *testing.T) {
	repo := &fakeOutboxRepo{claimResult: []model.MessageOutbox{
		outboxItem(1, `{"event":"ok"}`, 0),
		outboxItem(2, `{"event":"boom"}`, 0),
		outboxItem(3, `{"event":"ok2"}`, 0),
	}}
	pub := newFakePublisher()
	pub.failPayloads[`{"event":"boom"}`] = true
	svc := newOutboxForTest(repo, pub)

	svc.publishDue(context.Background())

	// 成功的两条 → MarkPublished；失败的一条 → MarkFailed
	if len(repo.publishedIDs) != 2 {
		t.Errorf("MarkPublished ids = %v, want 2 条", repo.publishedIDs)
	}
	if len(repo.failedCalls) != 1 || repo.failedCalls[0].id != 2 {
		t.Errorf("MarkFailed calls = %+v, want id=2 一条", repo.failedCalls)
	}
}

func TestOutboxPublishDue_ClaimError_NoCalls(t *testing.T) {
	repo := &fakeOutboxRepo{claimErr: errors.New("db down")}
	pub := newFakePublisher()
	svc := newOutboxForTest(repo, pub)

	svc.publishDue(context.Background())

	if len(repo.publishedIDs) != 0 || len(repo.failedCalls) != 0 || len(pub.published) != 0 {
		t.Errorf("ClaimDue 失败时不应有任何发布/标记动作")
	}
}

// TestBackoffDuration 表驱动：指数退避 1s 起步、1<<retryCount、6 次后封顶 64s。
func TestBackoffDuration(t *testing.T) {
	tests := []struct {
		retryCount int
		want       time.Duration
	}{
		{retryCount: 0, want: time.Second},
		{retryCount: 1, want: 2 * time.Second},
		{retryCount: 2, want: 4 * time.Second},
		{retryCount: 3, want: 8 * time.Second},
		{retryCount: 6, want: 64 * time.Second},
		{retryCount: 7, want: 64 * time.Second},  // 封顶
		{retryCount: 10, want: 64 * time.Second}, // 封顶
		{retryCount: -1, want: time.Second},      // 非法值归一到 1s
	}

	for _, tt := range tests {
		t.Run("retry_"+strconv.Itoa(tt.retryCount), func(t *testing.T) {
			got := backoffDuration(tt.retryCount)
			if got != tt.want {
				t.Errorf("backoffDuration(%d) = %v, want %v", tt.retryCount, got, tt.want)
			}
		})
	}
}
