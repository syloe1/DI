-- 000001_init.up.sql
-- go-admin 初始表结构（与 internal/domain/model 对齐，gorm MySQL 方言）

CREATE TABLE IF NOT EXISTS users (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    created_at  DATETIME(3)     NULL,
    updated_at  DATETIME(3)     NULL,
    deleted_at  DATETIME(3)     NULL,
    username    VARCHAR(32)     NOT NULL,
    password    VARCHAR(64)     NOT NULL,
    role        VARCHAR(16)     DEFAULT 'user',
    PRIMARY KEY (id),
    UNIQUE INDEX idx_users_username (username),
    INDEX idx_users_deleted_at (deleted_at)
);

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

CREATE TABLE IF NOT EXISTS messages (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    created_at  DATETIME(3)     NULL,
    updated_at  DATETIME(3)     NULL,
    deleted_at  DATETIME(3)     NULL,
    from_uid    BIGINT UNSIGNED NOT NULL,
    to_uid      BIGINT UNSIGNED NOT NULL,
    content     TEXT            NOT NULL,
    is_read     BOOLEAN         DEFAULT FALSE,
    PRIMARY KEY (id),
    INDEX idx_from_to (from_uid, to_uid),
    INDEX idx_messages_deleted_at (deleted_at)
);

CREATE TABLE IF NOT EXISTS chat_groups (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    created_at  DATETIME(3)     NULL,
    updated_at  DATETIME(3)     NULL,
    deleted_at  DATETIME(3)     NULL,
    name        VARCHAR(100)    NOT NULL,
    owner_uid   BIGINT UNSIGNED NOT NULL,
    status      VARCHAR(20)     NOT NULL DEFAULT 'normal',
    PRIMARY KEY (id),
    INDEX idx_chat_groups_status (status),
    INDEX idx_chat_groups_deleted_at (deleted_at)
);

CREATE TABLE IF NOT EXISTS chat_group_members (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    created_at  DATETIME(3)     NULL,
    updated_at  DATETIME(3)     NULL,
    deleted_at  DATETIME(3)     NULL,
    group_id    BIGINT UNSIGNED NOT NULL,
    user_id     BIGINT UNSIGNED NOT NULL,
    role        VARCHAR(20)     NOT NULL DEFAULT 'member',
    status      VARCHAR(20)     NOT NULL DEFAULT 'active',
    joined_at   DATETIME(3)     NOT NULL,
    left_at     DATETIME(3)     NULL,
    PRIMARY KEY (id),
    UNIQUE INDEX idx_group_user (group_id, user_id),
    INDEX idx_chat_group_members_user_id (user_id),
    INDEX idx_chat_group_members_role (role),
    INDEX idx_chat_group_members_status (status),
    INDEX idx_chat_group_members_deleted_at (deleted_at)
);

CREATE TABLE IF NOT EXISTS chat_group_invitations (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    created_at  DATETIME(3)     NULL,
    updated_at  DATETIME(3)     NULL,
    deleted_at  DATETIME(3)     NULL,
    group_id    BIGINT UNSIGNED NOT NULL,
    inviter_uid BIGINT UNSIGNED NOT NULL,
    invitee_uid BIGINT UNSIGNED NOT NULL,
    status      VARCHAR(20)     NOT NULL DEFAULT 'pending',
    expired_at  DATETIME(3)     NOT NULL,
    handled_at  DATETIME(3)     NULL,
    PRIMARY KEY (id),
    INDEX idx_chat_group_invitations_group_id (group_id),
    INDEX idx_chat_group_invitations_inviter_uid (inviter_uid),
    INDEX idx_chat_group_invitations_invitee_uid (invitee_uid),
    INDEX idx_chat_group_invitations_status (status),
    INDEX idx_chat_group_invitations_expired_at (expired_at),
    INDEX idx_chat_group_invitations_deleted_at (deleted_at)
);

CREATE TABLE IF NOT EXISTS chat_group_join_requests (
    id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    created_at   DATETIME(3)     NULL,
    updated_at   DATETIME(3)     NULL,
    deleted_at   DATETIME(3)     NULL,
    group_id     BIGINT UNSIGNED NOT NULL,
    user_id      BIGINT UNSIGNED NOT NULL,
    reason       VARCHAR(255)    NULL,
    status       VARCHAR(20)     NOT NULL DEFAULT 'pending',
    reviewer_uid BIGINT UNSIGNED NULL,
    reviewed_at  DATETIME(3)     NULL,
    expired_at   DATETIME(3)     NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_chat_group_join_requests_group_id (group_id),
    INDEX idx_chat_group_join_requests_user_id (user_id),
    INDEX idx_chat_group_join_requests_status (status),
    INDEX idx_chat_group_join_requests_reviewer_uid (reviewer_uid),
    INDEX idx_chat_group_join_requests_expired_at (expired_at),
    INDEX idx_chat_group_join_requests_deleted_at (deleted_at)
);

CREATE TABLE IF NOT EXISTS chat_group_messages (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    created_at  DATETIME(3)     NULL,
    updated_at  DATETIME(3)     NULL,
    deleted_at  DATETIME(3)     NULL,
    group_id    BIGINT UNSIGNED NOT NULL,
    sender_uid  BIGINT UNSIGNED NOT NULL,
    content     TEXT            NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_chat_group_messages_group_id (group_id),
    INDEX idx_chat_group_messages_sender_uid (sender_uid),
    INDEX idx_chat_group_messages_deleted_at (deleted_at)
);

CREATE TABLE IF NOT EXISTS message_outboxes (
    id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    created_at   DATETIME(3)     NULL,
    updated_at   DATETIME(3)     NULL,
    deleted_at   DATETIME(3)     NULL,
    event_type   VARCHAR(64)     NOT NULL,
    aggregate_id VARCHAR(64)     NOT NULL,
    payload      JSON            NOT NULL,
    status       VARCHAR(20)     NOT NULL DEFAULT 'pending',
    retry_count  BIGINT          NOT NULL DEFAULT 0,
    next_retry_at DATETIME(3)    NOT NULL,
    claimed_at   DATETIME(3)     NULL,
    claimed_by   VARCHAR(128)    NULL,
    published_at DATETIME(3)     NULL,
    last_error   TEXT            NULL,
    PRIMARY KEY (id),
    INDEX idx_outbox_status_retry (event_type, status, next_retry_at, claimed_at),
    INDEX idx_message_outboxes_aggregate_id (aggregate_id),
    INDEX idx_message_outboxes_claimed_by (claimed_by),
    INDEX idx_message_outboxes_deleted_at (deleted_at)
);
