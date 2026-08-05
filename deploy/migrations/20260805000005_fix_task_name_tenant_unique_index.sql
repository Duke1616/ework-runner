-- +goose Up
ALTER TABLE tasks
    DROP INDEX uniq_idx_name_tenant,
    ADD UNIQUE INDEX uniq_idx_name_tenant (tenant_id, name);

-- +goose Down
ALTER TABLE tasks
    DROP INDEX uniq_idx_name_tenant,
    ADD UNIQUE INDEX uniq_idx_name_tenant (name);
