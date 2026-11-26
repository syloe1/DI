-- 000003_message_indexes.down.sql
-- 回滚 000003 的组合索引。

DROP INDEX idx_messages_to_from_read ON messages;
DROP INDEX idx_messages_from_to_created ON messages;
DROP INDEX idx_group_messages_group_created ON chat_group_messages;
DROP INDEX idx_group_messages_group_id ON chat_group_messages;
DROP INDEX idx_group_members_user_status ON chat_group_members;
