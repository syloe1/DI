// Command seed 为演示环境写入种子数据，并清理评审/测试遗留账号。
//
// 用途：解决「新注册账号登录后是空应用，没法给面试官演示」的问题。
// 写入内容（幂等，可重复执行）：
//   - 4 个演示用户：demo / alice / bob / carol（默认密码 Demo@123456）
//   - 1 个演示群「DI 产品体验群」，demo 为群主，alice 为管理员
//   - 12 条群聊历史 + 10 条私聊会话（含未读红点）
//
// 清理（需要显式 -yes 确认）：
//
//	-purge-users-prefix review_      删除 review_ 前缀的测试账号及其群/消息
//	-purge-dissolved-groups          删除所有已解散群及其成员/消息
//
// 用法：
//
//	# 本地（与 server 使用同一套 APP_* 环境变量，或改 config.yaml）
//	go run ./cmd/seed
//
//	# 服务器（复用 systemd 的环境变量文件）
//	set -a; . /opt/go-admin/go-admin.env; set +a
//	/opt/go-admin/seed
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"di/internal/config"
	"di/internal/domain/model"
	"di/internal/infra"
	"di/internal/telemetry"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	demoGroupName = "DI 产品体验群"
	defaultPasswd = "Demo@123456"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	passwd := flag.String("demo-password", defaultPasswd, "演示账号密码（demo/alice/bob/carol 共用）")
	purgeUsersPrefix := flag.String("purge-users-prefix", "", "清理指定用户名前缀的测试账号（如 review_）")
	purgeDissolved := flag.Bool("purge-dissolved-groups", false, "清理所有已解散群及其成员/消息")
	yes := flag.Bool("yes", false, "确认执行清理类操作")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config failed: %v\n", err)
		os.Exit(1)
	}
	if _, err := telemetry.InitLogger(cfg.GetLoggerConfig().Level, cfg.GetLoggerConfig().Format); err != nil {
		fmt.Fprintf(os.Stderr, "init logger failed: %v\n", err)
		os.Exit(1)
	}
	defer telemetry.SyncLogger()

	db, err := infra.InitMysql(cfg.GetMysqlConfig())
	if err != nil {
		fatal("init mysql failed: %v", err)
	}
	// seed 只关心最终结果，屏蔽 GORM 的 record not found 噪音日志
	db.Logger = db.Logger.LogMode(logger.Silent)
	sqlDB, err := db.DB()
	if err != nil {
		fatal("get sql db failed: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()

	if err := infra.RunMigrations(db); err != nil {
		fatal("run migrations failed: %v", err)
	}

	// ---------- 清理（显式确认才执行） ----------
	if *purgeUsersPrefix != "" || *purgeDissolved {
		if !*yes {
			fmt.Println("检测到清理参数，但未加 -yes，未执行任何删除。预览：")
			if *purgeUsersPrefix != "" {
				var users []model.User
				db.Unscoped().Where("username LIKE ?", *purgeUsersPrefix+"%").Find(&users)
				fmt.Printf("  - 将删除 %d 个 %s* 账号\n", len(users), *purgeUsersPrefix)
			}
			if *purgeDissolved {
				var count int64
				db.Unscoped().Model(&model.ChatGroup{}).Where("status = ?", model.ChatGroupStatusDissolved).Count(&count)
				fmt.Printf("  - 将删除 %d 个已解散群\n", count)
			}
			os.Exit(0)
		}
		if *purgeUsersPrefix != "" {
			n, err := purgeUsersByPrefix(db, *purgeUsersPrefix)
			if err != nil {
				fatal("purge users failed: %v", err)
			}
			fmt.Printf("已清理 %d 个 %s* 测试账号（含其群/消息）\n", n, *purgeUsersPrefix)
		}
		if *purgeDissolved {
			n, err := purgeDissolvedGroups(db)
			if err != nil {
				fatal("purge dissolved groups failed: %v", err)
			}
			fmt.Printf("已清理 %d 个已解散群\n", n)
		}
	}

	// ---------- 种子数据 ----------
	usernames := []string{"demo", "alice", "bob", "carol"}
	users := make(map[string]*model.User, len(usernames))
	for _, name := range usernames {
		user, created, err := ensureUser(db, name, *passwd)
		if err != nil {
			fatal("ensure user %s failed: %v", name, err)
		}
		users[name] = user
		if created {
			fmt.Printf("创建用户 %-6s id=%d\n", name, user.ID)
		} else {
			fmt.Printf("用户已存在 %-6s id=%d（跳过）\n", name, user.ID)
		}
	}

	group, created, err := ensureGroup(db, demoGroupName, users["demo"].ID)
	if err != nil {
		fatal("ensure demo group failed: %v", err)
	}
	if created {
		fmt.Printf("创建演示群「%s」id=%d\n", group.Name, group.ID)
	} else {
		fmt.Printf("演示群已存在「%s」id=%d（跳过）\n", group.Name, group.ID)
	}

	members := []struct {
		name string
		role string
	}{
		{"demo", model.ChatGroupMemberRoleOwner},
		{"alice", model.ChatGroupMemberRoleAdmin},
		{"bob", model.ChatGroupMemberRoleMember},
		{"carol", model.ChatGroupMemberRoleMember},
	}
	for _, m := range members {
		if err := ensureMember(db, group.ID, users[m.name].ID, m.role); err != nil {
			fatal("ensure member %s failed: %v", m.name, err)
		}
	}

	groupMsgCount, err := countGroupMessages(db, group.ID)
	if err != nil {
		fatal("count group messages failed: %v", err)
	}
	if groupMsgCount == 0 {
		if err := seedGroupMessages(db, group.ID, users); err != nil {
			fatal("seed group messages failed: %v", err)
		}
		fmt.Println("写入 12 条群聊历史消息")
	} else {
		fmt.Printf("群消息已存在 %d 条（跳过）\n", groupMsgCount)
	}

	privateCount, err := countPrivateMessages(db, users["demo"].ID)
	if err != nil {
		fatal("count private messages failed: %v", err)
	}
	if privateCount == 0 {
		if err := seedPrivateMessages(db, users); err != nil {
			fatal("seed private messages failed: %v", err)
		}
		fmt.Println("写入 10 条私聊历史消息（含未读）")
	} else {
		fmt.Printf("私聊消息已存在 %d 条（跳过）\n", privateCount)
	}

	fmt.Println()
	fmt.Println("✅ 种子数据就绪。演示入口：")
	fmt.Println("   前端登录页 →「体验 Demo」按钮（免密，需 demo.enabled=true）")
	fmt.Printf("   手动登录：demo / %s\n", *passwd)
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func findUserByName(db *gorm.DB, username string) (*model.User, error) {
	var user model.User
	if err := db.Where("username = ?", username).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func ensureUser(db *gorm.DB, username, password string) (*model.User, bool, error) {
	user, err := findUserByName(db, username)
	if err == nil {
		return user, false, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, false, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, false, err
	}
	user = &model.User{Username: username, Password: string(hash), Role: model.RoleUser}
	if err := db.Create(user).Error; err != nil {
		return nil, false, err
	}
	return user, true, nil
}

func ensureGroup(db *gorm.DB, name string, ownerUID uint) (*model.ChatGroup, bool, error) {
	var group model.ChatGroup
	err := db.Where("name = ?", name).First(&group).Error
	if err == nil {
		return &group, false, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, false, err
	}
	group = model.ChatGroup{Name: name, OwnerUID: ownerUID, Status: model.ChatGroupStatusNormal}
	if err := db.Create(&group).Error; err != nil {
		return nil, false, err
	}
	return &group, true, nil
}

func ensureMember(db *gorm.DB, groupID, userID uint, role string) error {
	var member model.ChatGroupMember
	err := db.Where("group_id = ? AND user_id = ?", groupID, userID).First(&member).Error
	if err == nil {
		if member.Status != model.ChatGroupMemberStatusActive || member.Role != role {
			return db.Model(&member).Updates(map[string]interface{}{
				"status":  model.ChatGroupMemberStatusActive,
				"role":    role,
				"left_at": nil,
			}).Error
		}
		return nil
	}
	if err != gorm.ErrRecordNotFound {
		return err
	}
	member = model.ChatGroupMember{
		GroupID:  groupID,
		UserID:   userID,
		Role:     role,
		Status:   model.ChatGroupMemberStatusActive,
		JoinedAt: time.Now().Add(-3 * time.Hour),
	}
	return db.Create(&member).Error
}

func countGroupMessages(db *gorm.DB, groupID uint) (int64, error) {
	var count int64
	err := db.Model(&model.ChatGroupMessage{}).Where("group_id = ?", groupID).Count(&count).Error
	return count, err
}

func seedGroupMessages(db *gorm.DB, groupID uint, users map[string]*model.User) error {
	dialogue := []struct {
		sender  string
		content string
	}{
		{"demo", "欢迎来到 DI 实时通讯 Demo 👋"},
		{"alice", "这里是群聊：发消息、实时推送、历史记录都在。"},
		{"bob", "消息先写 MySQL + Outbox，再经 RabbitMQ 广播到各实例。"},
		{"carol", "断线重连后会自动按游标补拉漏掉的消息。"},
		{"demo", "左侧「消息」里还有私聊会话，可以直接回复。"},
		{"alice", "输入框 Enter 发送，Shift+Enter 换行。"},
		{"bob", "刷新页面后发送者名字也不会再变成「用户13」了。"},
		{"carol", "右侧成员面板能看到角色和在线状态。"},
		{"demo", "这是预置的体验数据，可用 cmd/seed 重复生成。"},
		{"alice", "WebSocket 心跳 30s，在线状态 90s TTL。"},
		{"bob", "点右上角头像可以改密码 / 退出登录。"},
		{"carol", "祝面试顺利 🚀"},
	}

	base := time.Now().Add(-2 * time.Hour)
	messages := make([]model.ChatGroupMessage, 0, len(dialogue))
	for i, item := range dialogue {
		sender := users[item.sender]
		ts := base.Add(time.Duration(i) * 8 * time.Minute)
		messages = append(messages, model.ChatGroupMessage{
			Model:     gorm.Model{CreatedAt: ts, UpdatedAt: ts},
			GroupID:   groupID,
			SenderUID: sender.ID,
			Content:   item.content,
		})
	}
	return db.Create(&messages).Error
}

func countPrivateMessages(db *gorm.DB, demoUID uint) (int64, error) {
	var count int64
	err := db.Model(&model.Message{}).
		Where("from_uid = ? OR to_uid = ?", demoUID, demoUID).
		Count(&count).Error
	return count, err
}

func seedPrivateMessages(db *gorm.DB, users map[string]*model.User) error {
	type msg struct {
		from string
		to   string
		text string
		read bool
	}
	dialogue := []msg{
		{"alice", "demo", "在吗？想确认一下群聊的发送者名称显示。", false},
		{"demo", "alice", "在的，后端 DTO 已经直接带 username 了。", true},
		{"alice", "demo", "好的，我刷新页面再验证一下。", false},
		{"demo", "alice", "顺便试下断线重连的补偿。", true},
		{"bob", "demo", "私聊也能实时收到 WebSocket 推送吗？", false},
		{"demo", "bob", "能，HTTP 落库后会推给在线接收方。", true},
		{"bob", "demo", "收到，红点未读数也对上了。", false},
		{"carol", "demo", "登录页的「体验 Demo」入口是你加的吗？", false},
		{"demo", "carol", "对，面试官点一下就能看到完整数据。", true},
		{"carol", "demo", "很实用，比空应用好多了 👍", false},
	}

	base := time.Now().Add(-110 * time.Minute)
	messages := make([]model.Message, 0, len(dialogue))
	for i, item := range dialogue {
		ts := base.Add(time.Duration(i) * 10 * time.Minute)
		messages = append(messages, model.Message{
			Model:   gorm.Model{CreatedAt: ts, UpdatedAt: ts},
			FromUID: users[item.from].ID,
			ToUID:   users[item.to].ID,
			Content: item.text,
			IsRead:  item.read,
		})
	}
	return db.Create(&messages).Error
}

// purgeUsersByPrefix 删除指定前缀的测试账号及其关联数据（硬删除，明确用于评审数据清理）。
func purgeUsersByPrefix(db *gorm.DB, prefix string) (int, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return 0, nil
	}
	var users []model.User
	if err := db.Unscoped().Where("username LIKE ?", prefix+"%").Find(&users).Error; err != nil {
		return 0, err
	}

	for _, user := range users {
		if err := db.Unscoped().Where("from_uid = ? OR to_uid = ?", user.ID, user.ID).Delete(&model.Message{}).Error; err != nil {
			return 0, err
		}

		var ownedGroups []model.ChatGroup
		if err := db.Unscoped().Where("owner_uid = ?", user.ID).Find(&ownedGroups).Error; err != nil {
			return 0, err
		}
		for _, group := range ownedGroups {
			if err := purgeGroupCascade(db, group.ID); err != nil {
				return 0, err
			}
		}

		if err := db.Unscoped().Where("user_id = ?", user.ID).Delete(&model.ChatGroupMember{}).Error; err != nil {
			return 0, err
		}
		if err := db.Unscoped().Where("inviter_uid = ? OR invitee_uid = ?", user.ID, user.ID).Delete(&model.ChatGroupInvitation{}).Error; err != nil {
			return 0, err
		}
		if err := db.Unscoped().Delete(&model.User{}, user.ID).Error; err != nil {
			return 0, err
		}
	}
	return len(users), nil
}

// purgeDissolvedGroups 删除所有已解散群及其成员/消息（群主可见的归档数据清理）。
func purgeDissolvedGroups(db *gorm.DB) (int, error) {
	var groups []model.ChatGroup
	if err := db.Unscoped().Where("status = ?", model.ChatGroupStatusDissolved).Find(&groups).Error; err != nil {
		return 0, err
	}
	for _, group := range groups {
		if err := purgeGroupCascade(db, group.ID); err != nil {
			return 0, err
		}
	}
	return len(groups), nil
}

func purgeGroupCascade(db *gorm.DB, groupID uint) error {
	if err := db.Unscoped().Where("group_id = ?", groupID).Delete(&model.ChatGroupMessage{}).Error; err != nil {
		return err
	}
	if err := db.Unscoped().Where("group_id = ?", groupID).Delete(&model.ChatGroupMember{}).Error; err != nil {
		return err
	}
	if err := db.Unscoped().Where("group_id = ?", groupID).Delete(&model.ChatGroupInvitation{}).Error; err != nil {
		return err
	}
	if err := db.Unscoped().Where("group_id = ?", groupID).Delete(&model.ChatGroupJoinRequest{}).Error; err != nil {
		return err
	}
	return db.Unscoped().Delete(&model.ChatGroup{}, groupID).Error
}
