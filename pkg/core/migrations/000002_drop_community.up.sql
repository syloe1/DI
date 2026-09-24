-- 000002_drop_community.up.sql
-- 项目收敛：砍掉内容社区模块（帖子/评论/点赞点踩收藏分享/关注拉黑关系），
-- 定位聚焦实时 IM（单聊 + 群聊 + Outbox 可靠投递）。

DROP TABLE IF EXISTS shares;
DROP TABLE IF EXISTS collects;
DROP TABLE IF EXISTS dislikes;
DROP TABLE IF EXISTS likes;
DROP TABLE IF EXISTS comments;
DROP TABLE IF EXISTS posts;
DROP TABLE IF EXISTS user_relations;
