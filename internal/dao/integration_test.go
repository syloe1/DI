//go:build integration

package dao

import (
	"os"
	"testing"
	"time"

	"go-admin/internal/domain/model"
	"go-admin/pkg/core"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 集成测试：连接真实 MySQL（docker 容器），验证 DAO 的 SQL 行为。
// 运行方式：go test -tags integration ./internal/dao/
// 连接串：环境变量 TEST_MYSQL_DSN，缺省连本机 33061 端口的 di-mysql-test 容器。
//
//	docker run -d --name di-mysql-test \
//	  -e MYSQL_ROOT_PASSWORD=test123 -e MYSQL_DATABASE=go_test \
//	  -p 33061:3306 mysql:8

var testDB *gorm.DB

func TestMain(m *testing.M) {
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		// multiStatements=true：golang-migrate 迁移 SQL 为多语句文件
		dsn = "root:test123@tcp(127.0.0.1:33061)/go_test?charset=utf8mb4&parseTime=True&loc=Local&multiStatements=true"
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		panic("connect test mysql failed: " + err.Error())
	}
	// 迁移前清理历史脏状态：失败过的迁移会在 schema_migrations 标记 dirty，
	// 测试库直接重建该表（生产环境 dirty 需人工修复，见 golang-migrate force）。
	if err := db.Exec("DROP TABLE IF EXISTS schema_migrations").Error; err != nil {
		panic("drop schema_migrations failed: " + err.Error())
	}
	// 走生产同款迁移路径（golang-migrate SQL），顺带验证迁移本身可用
	if err := core.RunMigrations(db); err != nil {
		panic("run migrations failed: " + err.Error())
	}
	testDB = db
	code := m.Run()
	os.Exit(code)
}

// cleanup 清空测试表，保证用例之间互不污染。
func cleanup(t *testing.T, tables ...string) {
	t.Helper()
	for _, table := range tables {
		if err := testDB.Exec("DELETE FROM " + table).Error; err != nil {
			t.Fatalf("cleanup %s failed: %v", table, err)
		}
	}
}

// newMessage 构造一条消息，CreatedAt 由外部指定以便断言排序。
func newMessage(from, to uint, content string, createdAt time.Time) *model.Message {
	return &model.Message{
		Model:     gorm.Model{CreatedAt: createdAt},
		FromUID:   from,
		ToUID:     to,
		Content:   content,
		IsRead:    false,
	}
}

// ==================== message DAO ====================

func TestMessageRepository_CreateAndPageQuery(t *testing.T) {
	cleanup(t, "messages")
	repo := NewGormMessageRepository(testDB)

	base := time.Now().Add(-time.Hour)
	var created []*model.Message
	for i := 0; i < 5; i++ {
		msg := newMessage(1, 2, "msg", base.Add(time.Duration(i)*time.Minute))
		if err := repo.CreateMessage(msg); err != nil {
			t.Fatalf("CreateMessage err = %v", err)
		}
		created = append(created, msg)
	}

	// 双向查询 + 分页（created_at DESC，最新在前）
	page1, err := repo.GetMessages(1, 2, 0, 2)
	if err != nil {
		t.Fatalf("GetMessages err = %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1 len = %d, want 2", len(page1))
	}
	// 最新一条（最后插入的）在前
	if page1[0].ID != created[4].ID {
		t.Errorf("page1[0].ID = %d, want %d（最新在前）", page1[0].ID, created[4].ID)
	}

	// 从对方视角同样能看到这 5 条
	all, err := repo.GetMessages(2, 1, 0, 10)
	if err != nil {
		t.Fatalf("GetMessages err = %v", err)
	}
	if len(all) != 5 {
		t.Errorf("all len = %d, want 5", len(all))
	}
}

func TestMessageRepository_MarkReadAndUnread(t *testing.T) {
	cleanup(t, "messages")
	repo := NewGormMessageRepository(testDB)

	now := time.Now()
	// A(1) 发给 B(2) 3 条未读；B 发给 A 1 条
	for i := 0; i < 3; i++ {
		if err := repo.CreateMessage(newMessage(1, 2, "a-to-b", now)); err != nil {
			t.Fatalf("CreateMessage err = %v", err)
		}
	}
	if err := repo.CreateMessage(newMessage(2, 1, "b-to-a", now)); err != nil {
		t.Fatalf("CreateMessage err = %v", err)
	}

	// B 视角：A 发的未读有 3 条
	ids, err := repo.FindUnreadMessageIDs(2, 1)
	if err != nil {
		t.Fatalf("FindUnreadMessageIDs err = %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("unread ids = %v, want 3 条", ids)
	}

	// 标记已读后：未读为 0，且 B→A 那条不受影响
	if err := repo.MarkMessagesAsRead(2, 1); err != nil {
		t.Fatalf("MarkMessagesAsRead err = %v", err)
	}
	unread, err := repo.CountUnreadMessages(2, 1)
	if err != nil {
		t.Fatalf("CountUnreadMessages err = %v", err)
	}
	if unread != 0 {
		t.Errorf("unread after mark = %d, want 0", unread)
	}
	// A 视角的未读（B 发给 A 的 1 条）仍是 1
	unreadA, err := repo.CountUnreadMessages(1, 2)
	if err != nil {
		t.Fatalf("CountUnreadMessages err = %v", err)
	}
	if unreadA != 1 {
		t.Errorf("A unread = %d, want 1", unreadA)
	}
}

func TestMessageRepository_SyncMessagesCursor(t *testing.T) {
	cleanup(t, "messages")
	repo := NewGormMessageRepository(testDB)

	now := time.Now()
	var created []*model.Message
	for i := 1; i <= 5; i++ {
		msg := newMessage(1, 2, "m", now)
		if err := repo.CreateMessage(msg); err != nil {
			t.Fatalf("CreateMessage err = %v", err)
		}
		created = append(created, msg)
	}
	ids := []uint{created[0].ID, created[1].ID, created[2].ID, created[3].ID, created[4].ID}

	// 游标指向第 2 条 → 只返回第 3、4 条（升序，limit 2）
	page1, err := repo.SyncMessages(2, ids[1], 2)
	if err != nil {
		t.Fatalf("SyncMessages err = %v", err)
	}
	if len(page1) != 2 || page1[0].ID != ids[2] || page1[1].ID != ids[3] {
		t.Fatalf("sync page1 ids = [%d %d], want [%d %d]", page1[0].ID, page1[1].ID, ids[2], ids[3])
	}

	// 游标推进到第 4 条 → 返回第 5 条
	page2, err := repo.SyncMessages(2, ids[3], 10)
	if err != nil {
		t.Fatalf("SyncMessages err = %v", err)
	}
	if len(page2) != 1 || page2[0].ID != ids[4] {
		t.Fatalf("sync page2 ids = [%d], want [%d]", page2[0].ID, ids[4])
	}
}

func TestMessageRepository_ConversationsAndBatchUnread(t *testing.T) {
	cleanup(t, "messages", "users")
	repo := NewGormMessageRepository(testDB)

	now := time.Now()
	// 会话：A(1)↔B(2) 2 条、C(3)→B(2) 1 条；再加一个只有历史没有新消息的 D(4)→B
	for i := 0; i < 2; i++ {
		if err := repo.CreateMessage(newMessage(1, 2, "a-b", now)); err != nil {
			t.Fatalf("CreateMessage err = %v", err)
		}
	}
	if err := repo.CreateMessage(newMessage(3, 2, "c-b", now)); err != nil {
		t.Fatalf("CreateMessage err = %v", err)
	}
	if err := repo.CreateMessage(newMessage(4, 2, "d-b", now)); err != nil {
		t.Fatalf("CreateMessage err = %v", err)
	}

	// 先造用户，验证 FindUsersByIDs 批量查询（收集实际 ID，不依赖自增值）
	u1 := &model.User{Username: "a"}
	u3 := &model.User{Username: "c"}
	for _, u := range []*model.User{u1, u3, &model.User{Username: "b"}, &model.User{Username: "d"}} {
		if err := testDB.Create(u).Error; err != nil {
			t.Fatalf("create user err = %v", err)
		}
	}

	convs, err := repo.GetConversations(2)
	if err != nil {
		t.Fatalf("GetConversations err = %v", err)
	}
	if len(convs) != 3 {
		t.Fatalf("conversations len = %d, want 3（A/B/C/D 四个 peer 聚合）", len(convs))
	}
	// 会话按最近消息排序：D、C、A 依次是最近一条消息的发送方（同 created_at 时按 id 反序）
	peerSet := map[uint]bool{}
	for _, c := range convs {
		peerSet[c.PeerUID] = true
	}
	for _, want := range []uint{1, 3, 4} {
		if !peerSet[want] {
			t.Errorf("conversations missing peer %d, got %v", want, peerSet)
		}
	}

	// N+1 修复的核心：批量未读统计
	unreadMap, err := repo.CountUnreadByPeers(2, []uint{1, 3, 4})
	if err != nil {
		t.Fatalf("CountUnreadByPeers err = %v", err)
	}
	if unreadMap[1] != 2 {
		t.Errorf("unread[A] = %d, want 2", unreadMap[1])
	}
	if unreadMap[3] != 1 {
		t.Errorf("unread[C] = %d, want 1", unreadMap[3])
	}
	if unreadMap[4] != 1 {
		t.Errorf("unread[D] = %d, want 1", unreadMap[4])
	}

	// 批量用户查询：只返回存在的 ID
	userMap, err := repo.FindUsersByIDs([]uint{u1.ID, u3.ID, 999999})
	if err != nil {
		t.Fatalf("FindUsersByIDs err = %v", err)
	}
	if len(userMap) != 2 || userMap[u1.ID] == nil || userMap[u3.ID] == nil {
		t.Errorf("userMap = %v, want 只含 u1(%d) 和 u3(%d)（999999 不存在）", userMap, u1.ID, u3.ID)
	}
}

// ==================== outbox DAO ====================

func newOutbox(status string, retryAt time.Time) *model.MessageOutbox {
	return &model.MessageOutbox{
		EventType:   "group_message",
		AggregateID: "g1",
		Payload:     `{"event":"group_message"}`,
		Status:      status,
		NextRetryAt: retryAt,
		ClaimedAt:   nil,
		ClaimedBy:   "",
		LastError:   "",
	}
}

func TestOutboxRepository_ClaimDue_ClaimAndSkipLocked(t *testing.T) {
	cleanup(t, "message_outboxes")
	repo := NewGormOutboxRepository(testDB)

	now := time.Now()
	item := newOutbox(model.MessageOutboxStatusPending, now.Add(-time.Minute))
	if err := testDB.Create(item).Error; err != nil {
		t.Fatalf("create outbox err = %v", err)
	}

	// 第一次抢占：pending 到期 → 抢到并标记 processing
	claimed, err := repo.ClaimDue(10, now, "inst-1", 30*time.Second)
	if err != nil {
		t.Fatalf("ClaimDue err = %v", err)
	}
	if len(claimed) != 1 || claimed[0].ID != item.ID {
		t.Fatalf("claimed = %+v, want 1 条 id=%d", claimed, item.ID)
	}
	if claimed[0].Status != model.MessageOutboxStatusProcessing || claimed[0].ClaimedBy != "inst-1" {
		t.Errorf("claimed status/by = %s/%s, want processing/inst-1", claimed[0].Status, claimed[0].ClaimedBy)
	}

	// 第二次抢占（processing 且未超时）：SKIP LOCKED 条件下不应再被抢
	again, err := repo.ClaimDue(10, now.Add(5*time.Second), "inst-2", 30*time.Second)
	if err != nil {
		t.Fatalf("ClaimDue err = %v", err)
	}
	if len(again) != 0 {
		t.Errorf("processing 未超时不应被二次抢占，got %d 条", len(again))
	}

	// 第三次（claimed_at 已超过 stale 阈值）：可被其他实例回收
	stale, err := repo.ClaimDue(10, now.Add(31*time.Second), "inst-2", 30*time.Second)
	if err != nil {
		t.Fatalf("ClaimDue err = %v", err)
	}
	if len(stale) != 1 || stale[0].ID != item.ID || stale[0].ClaimedBy != "inst-2" {
		t.Fatalf("stale reclaim = %+v, want id=%d claimed_by=inst-2", stale, item.ID)
	}
}

func TestOutboxRepository_FutureRetryNotClaimed(t *testing.T) {
	cleanup(t, "message_outboxes")
	repo := NewGormOutboxRepository(testDB)

	now := time.Now()
	item := newOutbox(model.MessageOutboxStatusPending, now.Add(5*time.Minute)) // 重试时间在未来
	if err := testDB.Create(item).Error; err != nil {
		t.Fatalf("create outbox err = %v", err)
	}

	claimed, err := repo.ClaimDue(10, now, "inst-1", 30*time.Second)
	if err != nil {
		t.Fatalf("ClaimDue err = %v", err)
	}
	if len(claimed) != 0 {
		t.Errorf("future retry 不应被抢占，got %d 条", len(claimed))
	}
}

func TestOutboxRepository_MarkFailed_RetryLimit(t *testing.T) {
	cleanup(t, "message_outboxes")
	repo := NewGormOutboxRepository(testDB)

	now := time.Now()
	item := newOutbox(model.MessageOutboxStatusProcessing, now.Add(-time.Minute))
	if err := testDB.Create(item).Error; err != nil {
		t.Fatalf("create outbox err = %v", err)
	}

	// 第 1 次失败：回到 pending，带下次重试时间
	if err := repo.MarkFailed(item.ID, 1, now.Add(2*time.Second), "mq down"); err != nil {
		t.Fatalf("MarkFailed err = %v", err)
	}
	var after model.MessageOutbox
	if err := testDB.First(&after, item.ID).Error; err != nil {
		t.Fatalf("reload err = %v", err)
	}
	if after.Status != model.MessageOutboxStatusPending {
		t.Errorf("status after retry1 = %s, want pending", after.Status)
	}
	if after.RetryCount != 1 || after.LastError != "mq down" {
		t.Errorf("retryCount/lastError = %d/%q, want 1/mq down", after.RetryCount, after.LastError)
	}

	// 第 10 次失败：超过重试上限 → 终态 failed
	if err := repo.MarkFailed(item.ID, 10, now.Add(64*time.Second), "give up"); err != nil {
		t.Fatalf("MarkFailed err = %v", err)
	}
	if err := testDB.First(&after, item.ID).Error; err != nil {
		t.Fatalf("reload err = %v", err)
	}
	if after.Status != model.MessageOutboxStatusFailed {
		t.Errorf("status after retry10 = %s, want failed", after.Status)
	}
}

func TestOutboxRepository_MarkPublished(t *testing.T) {
	cleanup(t, "message_outboxes")
	repo := NewGormOutboxRepository(testDB)

	now := time.Now()
	item := newOutbox(model.MessageOutboxStatusProcessing, now.Add(-time.Minute))
	if err := testDB.Create(item).Error; err != nil {
		t.Fatalf("create outbox err = %v", err)
	}

	if err := repo.MarkPublished(item.ID, now); err != nil {
		t.Fatalf("MarkPublished err = %v", err)
	}
	var after model.MessageOutbox
	if err := testDB.First(&after, item.ID).Error; err != nil {
		t.Fatalf("reload err = %v", err)
	}
	if after.Status != model.MessageOutboxStatusPublished {
		t.Errorf("status = %s, want published", after.Status)
	}
	if after.PublishedAt == nil || after.ClaimedBy != "" {
		t.Errorf("published_at 应写入且 claimed 清理：%+v", after)
	}
}
