package service

import (
	"context"
	"testing"
	"time"

	"go-admin/internal/dao"
	"go-admin/internal/domain/model"

	"gorm.io/gorm"
)

// fakeMessageRepo 内存实现 dao.MessageRepository，只保留测试关心的行为。
type fakeMessageRepo struct {
	syncMessages   []model.Message
	findUnreadIDs  []uint
	markReadCalled bool
	markReadError  error
	peerUsers      map[uint]*model.User
}

func (f *fakeMessageRepo) CreateMessage(message *model.Message) error { return nil }
func (f *fakeMessageRepo) FindMessageByID(id uint) (*model.Message, error) {
	return nil, nil
}
func (f *fakeMessageRepo) GetConversations(userID uint) ([]dao.Conversation, error) {
	return nil, nil
}
func (f *fakeMessageRepo) GetMessages(userID uint, peerID uint, offset int, limit int) ([]model.Message, error) {
	return nil, nil
}
func (f *fakeMessageRepo) MarkMessagesAsRead(userID uint, peerID uint) error {
	f.markReadCalled = true
	return f.markReadError
}
func (f *fakeMessageRepo) CountUnreadMessages(userID uint, peerID uint) (int64, error) {
	return 0, nil
}
func (f *fakeMessageRepo) FindUsersByIDs(ids []uint) (map[uint]*model.User, error) {
	return f.peerUsers, nil
}
func (f *fakeMessageRepo) CountUnreadByPeers(userID uint, peerIDs []uint) (map[uint]int64, error) {
	return map[uint]int64{}, nil
}
func (f *fakeMessageRepo) SyncMessages(userID uint, afterID uint, limit int) ([]model.Message, error) {
	out := make([]model.Message, 0, len(f.syncMessages))
	for _, m := range f.syncMessages {
		if m.ID > afterID {
			out = append(out, m)
		}
	}
	return out, nil
}
func (f *fakeMessageRepo) FindUnreadMessageIDs(userID uint, peerID uint) ([]uint, error) {
	return f.findUnreadIDs, nil
}
func (f *fakeMessageRepo) DeleteMessage(message *model.Message) error { return nil }
func (f *fakeMessageRepo) FindUserByID(userID uint) (*model.User, error) {
	if u, ok := f.peerUsers[userID]; ok {
		return u, nil
	}
	return nil, nil
}

func msg(id uint) model.Message {
	return model.Message{
		Model:   gorm.Model{ID: id, CreatedAt: time.Now()},
		FromUID: 1,
		ToUID:   2,
		Content: "hi",
		IsRead:  false,
	}
}

func user(id uint) *model.User {
	return &model.User{Model: gorm.Model{ID: id}, Username: "alice"}
}

// TestSyncMessages 表驱动：验证断线补偿 sync 的游标语义
// （afterID 过滤、limit 截断、limit+1 探测 has_more、lastID 取最大）。
func TestSyncMessages(t *testing.T) {
	all := []model.Message{msg(1), msg(2), msg(3), msg(4), msg(5)}

	tests := []struct {
		name      string
		afterID   uint
		limit     int
		wantCount int
		wantLast  uint
		wantMore  bool
	}{
		{name: "空库：游标不变，无更多", afterID: 100, limit: 50, wantCount: 0, wantLast: 100, wantMore: false},
		{name: "增量拉取：只返回 id 大于游标的消息", afterID: 2, limit: 50, wantCount: 3, wantLast: 5, wantMore: false},
		{name: "刚好 limit 条：无更多", afterID: 0, limit: 5, wantCount: 5, wantLast: 5, wantMore: false},
		{name: "超过 limit：截断并标记有更多", afterID: 0, limit: 3, wantCount: 3, wantLast: 3, wantMore: true},
		{name: "limit 超上限被钳制在 100", afterID: 0, limit: 1000, wantCount: 5, wantLast: 5, wantMore: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeMessageRepo{syncMessages: all}
			svc := &MessageService{messageRepo: repo, onMessagesRead: nil}

			res, err := svc.SyncMessages(context.Background(), 2, tt.afterID, tt.limit)
			if err != nil {
				t.Fatalf("SyncMessages err = %v", err)
			}
			if len(res.Messages) != tt.wantCount {
				t.Errorf("messages len = %d, want %d", len(res.Messages), tt.wantCount)
			}
			if res.LastID != tt.wantLast {
				t.Errorf("lastID = %d, want %d", res.LastID, tt.wantLast)
			}
			if res.HasMore != tt.wantMore {
				t.Errorf("hasMore = %v, want %v", res.HasMore, tt.wantMore)
			}
		})
	}
}

// TestGetMessageList_MarkReadTriggersReceipt 验证 S2-9 已读回执链路：
// 存在未读消息 → 调用 MarkMessagesAsRead + 触发 onMessagesRead(sender=peerID, reader=userID, ids)。
func TestGetMessageList_MarkReadTriggersReceipt(t *testing.T) {
	repo := &fakeMessageRepo{
		syncMessages:  []model.Message{msg(1)},
		findUnreadIDs: []uint{10, 11},
		peerUsers:     map[uint]*model.User{2: user(2)},
	}

	var gotSender, gotReader uint
	var gotIDs []uint
	svc := &MessageService{
		messageRepo: repo,
		onMessagesRead: func(senderUID uint, readerUID uint, messageIDs []uint) {
			gotSender = senderUID
			gotReader = readerUID
			gotIDs = messageIDs
		},
	}

	if _, err := svc.GetMessageList(context.Background(), 1, 2, 1, 20); err != nil {
		t.Fatalf("GetMessageList err = %v", err)
	}

	if !repo.markReadCalled {
		t.Error("MarkMessagesAsRead 未被调用")
	}
	if gotSender != 2 {
		t.Errorf("回调 sender = %d, want 2（消息发送方）", gotSender)
	}
	if gotReader != 1 {
		t.Errorf("回调 reader = %d, want 1（发起已读的用户）", gotReader)
	}
	if len(gotIDs) != 2 || gotIDs[0] != 10 || gotIDs[1] != 11 {
		t.Errorf("回调 messageIDs = %v, want [10 11]", gotIDs)
	}
}

// TestGetMessageList_NoUnread_NoCallback 验证无未读消息时不做多余动作。
func TestGetMessageList_NoUnread_NoCallback(t *testing.T) {
	repo := &fakeMessageRepo{
		syncMessages:  []model.Message{msg(1)},
		findUnreadIDs: nil,
		peerUsers:     map[uint]*model.User{2: user(2)},
	}

	callbackCalled := false
	svc := &MessageService{
		messageRepo: repo,
		onMessagesRead: func(uint, uint, []uint) {
			callbackCalled = true
		},
	}

	if _, err := svc.GetMessageList(context.Background(), 1, 2, 1, 20); err != nil {
		t.Fatalf("GetMessageList err = %v", err)
	}

	if repo.markReadCalled {
		t.Error("无未读消息时不应调用 MarkMessagesAsRead")
	}
	if callbackCalled {
		t.Error("无未读消息时不应触发已读回调")
	}
}
