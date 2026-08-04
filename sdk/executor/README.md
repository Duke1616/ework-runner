# Executor SDK

Executor SDK 将任务处理器 API 与 gRPC、制品缓存、执行状态等基础设施隔离。业务处理器只依赖 `TaskHandler` 和 `Context`。

Shell/Python 处理器支持的运行时环境变量、Runner 变量读取方式和制品路径约定见 [`internal/grpc/scripts/README.md`](../../internal/grpc/scripts/README.md)。

## 定义处理器

```go
package handler

import "github.com/Duke1616/etask/sdk/executor"

type SyncHandler struct{}

func (SyncHandler) Name() string { return "sync" }
func (SyncHandler) Desc() string { return "同步业务数据" }
func (SyncHandler) Metadata() []executor.Parameter { return nil }

func (SyncHandler) Run(ctx *executor.Context) error {
    action := ctx.Param("action")
    if action == "sync_db" {
        return db.Exec(ctx.Param("sql"))
    }
    return fmt.Errorf("不支持的操作: %s", action)
}
```

## 核心 API

### Context

- `Param`、`ParamInt`、`ParamInt64`、`ParamBool`：读取任务参数。
- `GetResolvedParam`：按参数绑定模式读取最终值。
- `SetResult`、`SetResults`、`AddResult`：写入结构化结果。
- `ArtifactRoots`：读取已准备的默认制品层和具名制品层。
- `Context`：获取承载取消信号和租户信息的原生上下文。
- `Log`、`Logger`：记录任务日志和系统日志。

### Executor

- `NewExecutor(config, registry)`：创建执行节点。
- `RegisterHandler(handlers...)`：注册任务处理器。
- `InitComponents()`：初始化缓存、客户端和 gRPC Server。
- `Server()`：返回供应用启动的 gRPC Server。

## 内部结构

```text
executor
├── context.go              # 公开任务上下文
├── handler.go              # Handler、Parameter 等公开契约
├── binding.go              # 参数绑定公开契约
├── config.go               # Executor 公开配置
├── executor.go             # Executor 构造入口
├── README.md
└── internal
    ├── task                # Context、日志和 Handler 注册实现
    ├── binding             # 调度侧参数绑定解析实现
    ├── runtime             # gRPC、PULL、执行编排和生命周期
    ├── artifactport        # SDK 与可选制品实现之间的接口
    └── execution           # 执行状态与取消生命周期
```

公开概念保留在根目录，只有实现细节进入 `internal`。业务代码统一导入 `sdk/executor`，不需要了解内部组合关系。

具体的制品缓存和本地物化属于 etask Executor 基础设施，位于 `internal/executor/artifact`，通过 `ArtifactPreparer` 接口注入 SDK。制品作用域、项目和租户规则由 `internal/domain` 与 `internal/service/artifact` 负责。
