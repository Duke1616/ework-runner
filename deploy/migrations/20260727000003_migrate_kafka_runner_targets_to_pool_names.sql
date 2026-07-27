-- +goose Up
-- Runner.target 自此统一保存 execution_pools.name。
-- 历史 KAFKA Runner 保存的是 metadata.topic，通过资源池元数据完成映射。
UPDATE runner AS r
JOIN execution_pools AS p
  ON p.transport = 'MQ'
 AND JSON_UNQUOTE(JSON_EXTRACT(p.metadata, '$.topic')) = r.target
SET r.target = p.name,
    r.utime = CAST(UNIX_TIMESTAMP(CURRENT_TIMESTAMP(3)) * 1000 AS UNSIGNED)
WHERE r.kind = 'KAFKA'
  AND r.target <> p.name;

-- +goose Down
UPDATE runner AS r
JOIN execution_pools AS p
  ON p.name = r.target
SET r.target = JSON_UNQUOTE(JSON_EXTRACT(p.metadata, '$.topic')),
    r.utime = CAST(UNIX_TIMESTAMP(CURRENT_TIMESTAMP(3)) * 1000 AS UNSIGNED)
WHERE r.kind = 'KAFKA'
  AND p.transport = 'MQ'
  AND JSON_UNQUOTE(JSON_EXTRACT(p.metadata, '$.topic')) IS NOT NULL
  AND JSON_UNQUOTE(JSON_EXTRACT(p.metadata, '$.topic')) <> '';
