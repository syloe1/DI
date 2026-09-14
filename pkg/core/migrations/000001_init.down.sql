-- 000001_init.down.sql
-- 回滚：删除全部表（逆序）

DROP TABLE IF EXISTS message_outboxes;
DROP TABLE IF EXISTS chat_group_messages;
DROP TABLE IF EXISTS chat_group_join_requests;
DROP TABLE IF EXISTS chat_group_invitations;
DROP TABLE IF EXISTS chat_group_members;
DROP TABLE IF EXISTS chat_groups;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS user_relations;
DROP TABLE IF EXISTS shares;
DROP TABLE IF EXISTS collects;
DROP TABLE IF EXISTS dislikes;
DROP TABLE IF EXISTS likes;
DROP TABLE IF EXISTS comments;
DROP TABLE IF EXISTS posts;
DROP TABLE IF EXISTS users;
