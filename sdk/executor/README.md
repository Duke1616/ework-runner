# Executor SDK

Executor SDK 将稳定的 Handler 契约与可选运行时分开：

- `sdk/executor`：Handler、Context 和参数契约。
- `sdk/executor/node`：标准 gRPC Executor 节点，包含 EGO、Registry、PUSH/PULL 和生命周期。
- `sdk/executor/engine`：Kafka 等自定义传输使用的进程内执行管线。
- `sdk/executor/artifact`：Node 和 Engine 可选的制品物化契约。

Shell/Python 处理器支持的环境变量、Runner 变量和制品路径约定见 [`internal/grpc/scripts/README.md`](../../internal/grpc/scripts/README.md)。

## 定义 Handler

Handler 只导入轻量根包：

```go
package handler

import (
    "fmt"

    "github.com/Duke1616/etask/sdk/executor"
)

type SyncHandler struct{}

func (SyncHandler) Name() string { return "sync" }
func (SyncHandler) Desc() string { return "同步业务数据" }
func (SyncHandler) Metadata() []executor.Parameter { return nil }

func (SyncHandler) Run(ctx *executor.Context) error {
    if ctx.Param("action") != "sync_db" {
        return fmt.Errorf("不支持的操作: %s", ctx.Param("action"))
    }
    ctx.Log("开始同步")
    ctx.SetResult("status", "ok")
    return nil
}
```

`Context` 主要能力：

- `Param`、`ParamInt`、`ParamInt64`、`ParamBool`：读取任务参数。
- `GetResolvedParam`：按 Handler 元数据解析参数绑定。
- `SetResult`、`SetResults`、`AddResult`：写入结构化结果。
- `ArtifactRoots`：读取已准备的默认层和具名制品层。
- `Log`、`Logger`：记录任务日志和结构化系统日志。
- `ReportProgress`：上报规范化到 0-100 的任务进度。

## 启动标准 Node

标准 gRPC Executor 显式导入 `node` 子包：

```go
import (
    "github.com/Duke1616/etask/sdk/executor/node"
    "github.com/gotomicro/ego"
)

exec, err := node.New(cfg, registry, SyncHandler{})
if err != nil {
    return err
}
return ego.New().Serve(exec).Run()
```

`node.Executor` 本身实现 `server.Server`。直接启动它可以在停止时取消 PULL 循环、等待执行中任务并关闭调度连接。

需要制品物化或手动装配时，使用细粒度入口：

```go
exec, err := node.NewExecutor(cfg, registry,
    node.WithArtifactPreparer(preparer),
)
if err != nil {
    return err
}
if err = exec.RegisterHandlers(handlers...); err != nil {
    return err
}
if err = exec.InitComponents(); err != nil {
    return err
}
return ego.New().Serve(exec).Run()
```

## 自定义传输

Kafka、本地队列或其他非 gRPC 传输使用 `engine` 子包：

```go
handlers := engine.NewHandlerRegistry()
if err := handlers.Register(myHandlers...); err != nil {
    return err
}
pipeline := engine.New(handlers, artifacts,
    engine.WithArtifactDownloader(downloader),
    engine.WithProgressReporter(progressReporter),
    engine.WithSystemLogger(systemLogger),
)
result, err := pipeline.Execute(ctx, engine.Command{
    /* task input */
    ExecutionLogger: executionLogger,
})
```

Engine 的 artifact downloader、progress reporter 和 system logger 是生命周期依赖，通过构造选项注入；单次任务面向用户的 `ExecutionLogger` 放在 `Command` 中。这些契约都不依赖 gRPC 或 EGO，具体传输和日志实现由 adapter 提供。调度端 Codebook、Runner 等参数绑定解析属于 etask 内部业务，不是 Executor SDK 扩展点。

## 包结构

```text
executor
├── context.go              # Handler 上下文和端口
├── handler.go              # Handler、Parameter 等稳定契约
├── artifact                # 可选制品契约
│   └── grpc               # gRPC 制品下载 adapter
├── engine                  # 自定义传输执行管线
├── logging/egolog          # EGO 日志 adapter
├── node                    # 标准 gRPC Executor 运行时
└── internal
    ├── task                # Context 和 Handler 注册实现
    ├── engine              # 单一执行核心
    ├── runtime             # gRPC、PULL、状态和生命周期
    └── execution           # 执行状态与取消管理
```
