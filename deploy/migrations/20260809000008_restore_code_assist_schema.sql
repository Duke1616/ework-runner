-- +goose Up
-- 上一条重置迁移删除旧数据后，必须显式重建 CodeAssist 当前仍在使用的表。
-- 不能依赖启动前执行的 AutoMigrate，因为 Goose 迁移在它之后运行。
CREATE TABLE IF NOT EXISTS ai_conversation (
    id BIGINT NOT NULL AUTO_INCREMENT,
    tenant_id BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '租户ID',
    user_id BIGINT UNSIGNED NOT NULL COMMENT '创建用户ID',
    project_id BIGINT NOT NULL COMMENT 'Codebook项目ID',
    title VARCHAR(128) NOT NULL COMMENT '会话标题',
    provider VARCHAR(32) NOT NULL COMMENT '模型供应商',
    model VARCHAR(128) NOT NULL COMMENT '模型名称',
    status VARCHAR(16) NOT NULL COMMENT '会话状态',
    run_token VARCHAR(64) NOT NULL DEFAULT '' COMMENT '当前生成租约令牌',
    ctime BIGINT NOT NULL COMMENT '创建时间',
    utime BIGINT NOT NULL COMMENT '更新时间',
    PRIMARY KEY (id),
    INDEX idx_ai_conversation_project (tenant_id, project_id),
    INDEX idx_ai_conversation_user_id (user_id),
    INDEX idx_ai_conversation_status (status)
);

CREATE TABLE IF NOT EXISTS ai_message (
    id BIGINT NOT NULL AUTO_INCREMENT,
    tenant_id BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '租户ID',
    conversation_id BIGINT NOT NULL COMMENT 'AI会话ID',
    role VARCHAR(16) NOT NULL COMMENT '消息角色',
    content LONGTEXT NOT NULL COMMENT '消息内容',
    status VARCHAR(16) NOT NULL COMMENT '生成状态',
    provider VARCHAR(32) NOT NULL DEFAULT '' COMMENT '模型供应商',
    model VARCHAR(128) NOT NULL DEFAULT '' COMMENT '模型名称',
    profile_id VARCHAR(64) NOT NULL DEFAULT '' COMMENT '协作配置标识',
    profile_version VARCHAR(32) NOT NULL DEFAULT '' COMMENT '协作配置版本',
    input_tokens BIGINT NOT NULL DEFAULT 0 COMMENT '输入Token数',
    output_tokens BIGINT NOT NULL DEFAULT 0 COMMENT '输出Token数',
    latency_millis BIGINT NOT NULL DEFAULT 0 COMMENT '响应耗时毫秒',
    error_message TEXT NOT NULL COMMENT '失败原因',
    ctime BIGINT NOT NULL COMMENT '创建时间',
    utime BIGINT NOT NULL COMMENT '更新时间',
    PRIMARY KEY (id),
    INDEX idx_ai_message_conversation (tenant_id, conversation_id),
    INDEX idx_ai_message_status (status)
);

CREATE TABLE IF NOT EXISTS ai_change_set (
    id BIGINT NOT NULL AUTO_INCREMENT,
    tenant_id BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '租户ID',
    conversation_id BIGINT NOT NULL COMMENT 'AI会话ID',
    message_id BIGINT NOT NULL COMMENT '模型消息ID',
    project_id BIGINT NOT NULL COMMENT 'Codebook项目ID',
    base_revision BIGINT NOT NULL COMMENT '生成时项目源码修订号',
    summary TEXT NOT NULL COMMENT '修改摘要',
    items JSON NOT NULL COMMENT '完整文件变更项',
    status VARCHAR(16) NOT NULL COMMENT '候选状态',
    ctime BIGINT NOT NULL COMMENT '创建时间',
    utime BIGINT NOT NULL COMMENT '更新时间',
    PRIMARY KEY (id),
    INDEX idx_ai_change_set_conversation (tenant_id, conversation_id),
    INDEX idx_ai_change_set_message_id (message_id),
    INDEX idx_ai_change_set_project_id (project_id),
    INDEX idx_ai_change_set_status (status)
);

-- +goose Down
DROP TABLE IF EXISTS ai_change_set;
DROP TABLE IF EXISTS ai_message;
DROP TABLE IF EXISTS ai_conversation;
