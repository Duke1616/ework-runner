-- +goose Up
-- CodeAssist 尚未正式投入使用，重构后直接清理旧会话结构和试用数据。
-- 服务启动时由 AutoMigrate 按新的 Profile 模型重新创建三张表。
DROP TABLE IF EXISTS ai_change_set;
DROP TABLE IF EXISTS ai_message;
DROP TABLE IF EXISTS ai_conversation;

-- +goose Down
-- 已删除的 AI 试用数据不可恢复；回滚代码时由 AutoMigrate 重建对应结构。
SELECT 1;
