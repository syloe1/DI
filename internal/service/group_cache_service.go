package service

import (
	"context"
	"fmt"
	"hash/fnv"
	"strconv"
	"time"

	"go-admin/internal/dao"
	"go-admin/internal/domain/model"

	"github.com/go-redis/redis/v8"
)

const (
	groupMemberCacheTTL = 10 * time.Minute
	//分16片  一片625
	groupOnlineShardNum = 16
)

type GroupCacheService struct {
	client    *redis.Client
	groupRepo dao.GroupRepository
	shards    int //用户分片数量
}

func NewGroupCacheService(client *redis.Client, groupRepo dao.GroupRepository) *GroupCacheService {
	return &GroupCacheService{
		client:    client,
		groupRepo: groupRepo,
		shards:    groupOnlineShardNum,
	}
}

// 先读 Redis 缓存，没有就查数据库，查到后自动写回缓存，下次直接用。
func (s *GroupCacheService) ActiveMemberIDs(ctx context.Context, groupID uint) ([]uint, error) {
	if s == nil || s.client == nil {
		return s.loadActiveMemberIDsFromDB(groupID)
	}

	key := groupMembersKey(groupID)
	//尝试读取缓存 Redis SMEMBERS 获取集合中所有成员
	values, err := s.client.SMembers(ctx, key).Result()
	if err != nil && err != redis.Nil {
		return nil, err
	}
	//缓存不存子啊
	if len(values) == 0 {
		//查DB
		ids, err := s.loadActiveMemberIDsFromDB(groupID)
		if err != nil {
			return nil, err
		}
		//DB有数据
		if len(ids) > 0 {
			//【】uint->interface
			members := make([]interface{}, 0, len(ids))
			for _, id := range ids {
				//无符号整数-> 10进制字符串
				members = append(members, strconv.FormatUint(uint64(id), 10))
			}
			//批量执行Redis命令
			pipe := s.client.TxPipeline()
			pipe.SAdd(ctx, key, members...)            //把成员写入集合
			pipe.Expire(ctx, key, groupMemberCacheTTL) //设置十分钟过期
			_, _ = pipe.Exec(ctx)                      //执行
		}
		return ids, nil
	}
	//Redis存的是[]string, 转成[]uint
	return parseUintSet(values), nil
}

func (s *GroupCacheService) RefreshActiveMembers(ctx context.Context, groupID uint) error {
	if s == nil || s.client == nil {
		return nil
	}

	ids, err := s.loadActiveMemberIDsFromDB(groupID)
	if err != nil {
		return err
	}

	key := groupMembersKey(groupID)
	//事务管道
	pipe := s.client.TxPipeline()
	pipe.Del(ctx, key)
	if len(ids) > 0 {
		members := make([]interface{}, 0, len(ids))
		for _, id := range ids {
			members = append(members, strconv.FormatUint(uint64(id), 10))
		}
		//写入集合
		pipe.SAdd(ctx, key, members...)
		pipe.Expire(ctx, key, groupMemberCacheTTL)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (s *GroupCacheService) AddUserOnlineGroups(ctx context.Context, userID uint) error {
	if s == nil || s.client == nil || userID == 0 {
		return nil
	}
	//查询用户加入了哪些群
	groups, err := s.groupRepo.ListGroupsByUserID(userID)
	if err != nil {
		return err
	}
	//ID->字符串
	uid := strconv.FormatUint(uint64(userID), 10)
	pipe := s.client.TxPipeline()
	for _, group := range groups {
		if group.Status != model.ChatGroupStatusNormal {
			continue
		}
		//写入群的在线分片
		pipe.SAdd(ctx, groupOnlineShardKey(group.GroupID, uid, s.shards), uid)
		//给在线状态设置过期时间
		pipe.Expire(ctx, groupOnlineBaseKey(group.GroupID), presenceTTL)
	}
	_, err = pipe.Exec(ctx)
	return err
}

// 用户下线： 从所在群的在线成员列表中移除自己
func (s *GroupCacheService) RemoveUserOnlineGroups(ctx context.Context, userID uint) error {
	if s == nil || s.client == nil || userID == 0 {
		return nil
	}

	groups, err := s.groupRepo.ListGroupsByUserID(userID)
	if err != nil {
		return err
	}
	//Redis只认字符串
	uid := strconv.FormatUint(uint64(userID), 10)
	pipe := s.client.TxPipeline()
	for _, group := range groups {
		//从对应分片中删除用户ID
		pipe.SRem(ctx, groupOnlineShardKey(group.GroupID, uid, s.shards), uid)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (s *GroupCacheService) OnlineMemberIDsByShard(ctx context.Context, groupID uint) ([][]uint, error) {
	result := make([][]uint, s.shards)
	//[分片1 ... 分片15]
	if s == nil || s.client == nil {
		return result, nil
	}

	for shard := 0; shard < s.shards; shard++ {
		//获取当前分片的Redis Key
		//看分片里面的在线用户
		values, err := s.client.SMembers(ctx, groupOnlineShardIndexKey(groupID, shard)).Result()
		if err != nil && err != redis.Nil {
			return nil, err
		}
		result[shard] = parseUintSet(values)
	}
	return result, nil
}

// 活跃 + 在线的用户，按分片返回
func (s *GroupCacheService) ActiveOnlineMemberIDsByShard(ctx context.Context, groupID uint) ([][]uint, error) {
	//获取活跃成员ID
	activeIDs, err := s.ActiveMemberIDs(ctx, groupID)
	if err != nil {
		return nil, err
	}
	//放Map里面, O(1)查找
	active := make(map[uint]struct{}, len(activeIDs))
	for _, id := range activeIDs {
		active[id] = struct{}{}
	}
	//获取所有在线成员的分片
	onlineShards, err := s.OnlineMemberIDsByShard(ctx, groupID)
	if err != nil {
		return nil, err
	}

	for shard, ids := range onlineShards {
		filtered := ids[:0]
		for _, id := range ids {
			if _, ok := active[id]; ok {
				filtered = append(filtered, id)
			}
		}
		onlineShards[shard] = filtered
	}
	return onlineShards, nil
}

func (s *GroupCacheService) loadActiveMemberIDsFromDB(groupID uint) ([]uint, error) {
	members, err := s.groupRepo.ListActiveMembers(groupID)
	if err != nil {
		return nil, err
	}
	//存放用户ID
	ids := make([]uint, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.UserID)
	}
	return ids, nil
}

// 生成群活跃成员缓存的Redis Key
func groupMembersKey(groupID uint) string {
	return fmt.Sprintf("group_members:%d", groupID)
}

// 查找群在线状态的基础Redis Key
func groupOnlineBaseKey(groupID uint) string {
	return fmt.Sprintf("group_online:%d", groupID)
}

// 用户哈希分片算法 同一个用户， 永远分配到同一个分片上
func groupOnlineShardKey(groupID uint, uid string, shards int) string {
	return groupOnlineShardIndexKey(groupID, int(hashString(uid)%uint32(shards)))
}

// 分片号 = 哈希值(用户ID) % 总分片数
func hashString(value string) uint32 {
	h := fnv.New32a() //哈希计算器
	_, _ = h.Write([]byte(value))
	return h.Sum32() //hash后的数字
}

func groupOnlineShardIndexKey(groupID uint, shard int) string {
	return fmt.Sprintf("group_online:%d:%d", groupID, shard)
}

func parseUintSet(values []string) []uint {
	ids := make([]uint, 0, len(values))
	for _, value := range values {
		id, err := strconv.ParseUint(value, 10, 32)
		if err == nil && id > 0 {
			ids = append(ids, uint(id))
		}
	}
	return ids
}
