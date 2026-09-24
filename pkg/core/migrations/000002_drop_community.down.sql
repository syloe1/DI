-- 000002_drop_community.down.sql
-- 回滚：恢复社区表结构（与 000001 初始定义一致）

CREATE TABLE IF NOT EXISTS posts (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    created_at  DATETIME(3)     NULL,
    updated_at  DATETIME(3)     NULL,
    deleted_at  DATETIME(3)     NULL,
    title       VARCHAR(128)    NOT NULL,
    content     TEXT            NOT NULL,
    is_public   BOOLEAN         NULL,
    status      VARCHAR(16)     DEFAULT 'published',
    publish_at  DATETIME(3)     NULL,
    topics      VARCHAR(255)    NULL,
    user_id     BIGINT UNSIGNED NOT NULL,
    username    VARCHAR(32)     NULL,
    images      VARCHAR(512)    NULL,
    PRIMARY KEY (id),
    INDEX idx_posts_user_id (user_id),
    INDEX idx_posts_deleted_at (deleted_at)
);

CREATE TABLE IF NOT EXISTS comments (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    created_at  DATETIME(3)     NULL,
    updated_at  DATETIME(3)     NULL,
    deleted_at  DATETIME(3)     NULL,
    post_id     BIGINT UNSIGNED NOT NULL,
    user_id     BIGINT UNSIGNED NOT NULL,
    username    VARCHAR(32)     NOT NULL,
    content     TEXT            NOT NULL,
    parent_id   BIGINT UNSIGNED DEFAULT 0,
    status      VARCHAR(255)    DEFAULT 'normal',
    PRIMARY KEY (id),
    INDEX idx_comments_deleted_at (deleted_at)
);

CREATE TABLE IF NOT EXISTS likes (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    created_at  DATETIME(3)     NULL,
    updated_at  DATETIME(3)     NULL,
    deleted_at  DATETIME(3)     NULL,
    user_id     BIGINT UNSIGNED NOT NULL,
    post_id     BIGINT UNSIGNED NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_user_post (user_id, post_id),
    INDEX idx_likes_deleted_at (deleted_at)
);

CREATE TABLE IF NOT EXISTS dislikes (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    created_at  DATETIME(3)     NULL,
    updated_at  DATETIME(3)     NULL,
    deleted_at  DATETIME(3)     NULL,
    user_id     BIGINT UNSIGNED NOT NULL,
    post_id     BIGINT UNSIGNED NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_user_post (user_id, post_id),
    INDEX idx_dislikes_deleted_at (deleted_at)
);

CREATE TABLE IF NOT EXISTS collects (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    created_at  DATETIME(3)     NULL,
    updated_at  DATETIME(3)     NULL,
    deleted_at  DATETIME(3)     NULL,
    user_id     BIGINT UNSIGNED NOT NULL,
    post_id     BIGINT UNSIGNED NOT NULL,
    remark      VARCHAR(128)    DEFAULT '',
    PRIMARY KEY (id),
    INDEX idx_user_post (user_id, post_id),
    INDEX idx_collects_deleted_at (deleted_at)
);

CREATE TABLE IF NOT EXISTS shares (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    created_at  DATETIME(3)     NULL,
    updated_at  DATETIME(3)     NULL,
    deleted_at  DATETIME(3)     NULL,
    user_id     BIGINT UNSIGNED NOT NULL,
    post_id     BIGINT UNSIGNED NOT NULL,
    platform    VARCHAR(32)     NOT NULL,
    share_url   VARCHAR(255)    NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_shares_user_id (user_id),
    INDEX idx_shares_post_id (post_id),
    INDEX idx_shares_deleted_at (deleted_at)
);

CREATE TABLE IF NOT EXISTS user_relations (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    created_at  DATETIME(3)     NULL,
    updated_at  DATETIME(3)     NULL,
    deleted_at  DATETIME(3)     NULL,
    from_uid    BIGINT UNSIGNED NOT NULL,
    to_uid      BIGINT UNSIGNED NOT NULL,
    type        VARCHAR(16)     NOT NULL,
    PRIMARY KEY (id),
    UNIQUE INDEX idx_relation_unique (from_uid, to_uid, type),
    INDEX idx_user_relations_deleted_at (deleted_at)
);
