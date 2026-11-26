-- 000002_message_indexes.up.sql
-- 组合索引（对应审计 P1：消息/群消息查询的组合过滤与排序）。
-- 注意：上线前需在真实数据分布下 EXPLAIN ANALYZE 验证；无效或冗余的索引应移除，
-- 不要在未验证的情况下全量保留（写放大 / 磁盘成本）。

-- 消息未读统计/更新：WHERE to_uid=? AND from_uid=? AND is_read=FALSE
CREATE INDEX idx_messages_to_from_read ON messages (to_uid, from_uid, is_read, id);

-- 会话分页：按会话过滤 + created_at 排序（覆盖 idx_from_to 的 (from_uid,to_uid) 前缀）
CREATE INDEX idx_messages_from_to_created ON messages (from_uid, to_uid, created_at, id);

-- 群消息分页：WHERE group_id=? ORDER BY created_at/id DESC
CREATE INDEX idx_group_messages_group_created ON chat_group_messages (group_id, created_at, id);
CREATE INDEX idx_group_messages_group_id ON chat_group_messages (group_id, id);

-- 群成员：按用户查其所在群（在线分组/我的群），过滤 status + 排序
CREATE INDEX idx_group_members_user_status ON chat_group_members (user_id, status, joined_at, group_id);
