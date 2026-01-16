# Executor SDK

极简 Executor SDK - 让你只关注业务逻辑,零样板代码。

## 快速开始

### 1. 创建业务处理函数

```go
package handler

import "github.com/Duke1616/ework-runner/sdk/executor"

// ProcessTask 处理任务的业务逻辑
func ProcessTask(ctx *executor.Context) error {
    // 获取参数
    action := ctx.Param("action")
    
    // 执行业务逻辑
    switch action {
    case "sync_db":
        sql := ctx.Param("sql")
        return db.Exec(sql)
        
    case "batch_process":
        items := loadItems()
        for i, item := range items {
            process(item)
            // 可选:上报进度
            ctx.ReportProgress((i+1) * 100 / len(items))
        }
        return nil
        
    default:
        return fmt.Errorf("unknown action: %s", action)
    }
}
```

### 2. 启动 Executor

```go
package main

import "github.com/Duke1616/ework-runner/sdk/executor"

func main() {
    cfg := &executor.Config{
        NodeID:        "cmdb-executor-001",
        ServiceName:   "cmdb",
        Addr:          "0.0.0.0:9020",
        EtcdEndpoints: []string{"localhost:2379"},
        ReporterAddr:  "127.0.0.1:9002",
    }
    
    exec := executor.MustNewExecutor(cfg)
    exec.RegisterHandler(handler.ProcessTask)
    exec.Start()  // 启动并阻塞
}
```

## API

### executor.Context

- `Param(key string) string` - 获取字符串参数
- `ParamInt(key string) int` - 获取整数参数
- `ParamInt64(key string) int64` - 获取 int64 参数
- `ParamBool(key string) bool` - 获取布尔参数
- `ReportProgress(progress int) error` - 上报进度(可选)
- `Logger() *elog.Component` - 获取日志

### executor.Executor

- `NewExecutor(cfg *Config) (*Executor, error)` - 创建 Executor
- `MustNewExecutor(cfg *Config) *Executor` - 创建 Executor(panic on error)
- `RegisterHandler(handler func(*Context) error) *Executor` - 注册处理函数
- `Start() error` - 启动并阻塞

## 设计原则

- **极简**: 用户只写业务逻辑,SDK 处理所有基础设施
- **可选进度**: ReportProgress 是可选的,不调用也OK
- **自动上报**: SDK 自动上报最终结果(成功/失败)
