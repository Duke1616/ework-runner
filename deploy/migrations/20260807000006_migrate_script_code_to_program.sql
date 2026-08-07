-- +goose Up
-- 旧脚本任务把 Codebook ID 存在 grpc_config.params.code，并用 metadata.code 声明绑定类型。
-- ProgramSpec 上线后，程序来源独立持久化，Handler 参数不再承载代码。
UPDATE tasks
SET program = JSON_OBJECT(
        'kind', 'INLINE',
        'inline', JSON_OBJECT(
            'codebookId', CAST(JSON_UNQUOTE(JSON_EXTRACT(grpc_config, '$.params.code')) AS UNSIGNED)
        )
    ),
    grpc_config = JSON_REMOVE(grpc_config, '$.params.code'),
    metadata = JSON_REMOVE(metadata, '$.code')
WHERE program IS NULL
  AND JSON_UNQUOTE(JSON_EXTRACT(grpc_config, '$.handlerName')) IN ('shell', 'python')
  AND JSON_UNQUOTE(JSON_EXTRACT(metadata, '$.code')) = 'codebook'
  AND JSON_UNQUOTE(JSON_EXTRACT(grpc_config, '$.params.code')) REGEXP '^[1-9][0-9]*$';

UPDATE tasks
SET program = JSON_OBJECT(
        'kind', 'INLINE',
        'inline', JSON_OBJECT(
            'code', JSON_UNQUOTE(JSON_EXTRACT(grpc_config, '$.params.code'))
        )
    ),
    grpc_config = JSON_REMOVE(grpc_config, '$.params.code'),
    metadata = JSON_REMOVE(metadata, '$.code')
WHERE program IS NULL
  AND JSON_UNQUOTE(JSON_EXTRACT(grpc_config, '$.handlerName')) IN ('shell', 'python')
  AND (
      JSON_UNQUOTE(JSON_EXTRACT(metadata, '$.code')) = 'static'
      OR JSON_EXTRACT(metadata, '$.code') IS NULL
  )
  AND COALESCE(JSON_UNQUOTE(JSON_EXTRACT(grpc_config, '$.params.code')), '') <> '';

-- +goose Down
UPDATE tasks
SET grpc_config = JSON_SET(
        grpc_config,
        '$.params.code',
        CAST(JSON_UNQUOTE(JSON_EXTRACT(program, '$.inline.codebookId')) AS CHAR)
    ),
    metadata = JSON_SET(COALESCE(metadata, JSON_OBJECT()), '$.code', 'codebook'),
    program = NULL
WHERE JSON_UNQUOTE(JSON_EXTRACT(program, '$.kind')) = 'INLINE'
  AND JSON_EXTRACT(program, '$.inline.codebookId') IS NOT NULL
  AND JSON_UNQUOTE(JSON_EXTRACT(grpc_config, '$.handlerName')) IN ('shell', 'python');

UPDATE tasks
SET grpc_config = JSON_SET(
        grpc_config,
        '$.params.code',
        JSON_UNQUOTE(JSON_EXTRACT(program, '$.inline.code'))
    ),
    metadata = JSON_SET(COALESCE(metadata, JSON_OBJECT()), '$.code', 'static'),
    program = NULL
WHERE JSON_UNQUOTE(JSON_EXTRACT(program, '$.kind')) = 'INLINE'
  AND JSON_EXTRACT(program, '$.inline.code') IS NOT NULL
  AND JSON_UNQUOTE(JSON_EXTRACT(grpc_config, '$.handlerName')) IN ('shell', 'python');
