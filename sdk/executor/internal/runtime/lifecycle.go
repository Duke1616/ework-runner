package runtime

// 本文件适配 ego Server 生命周期。

import (
	"context"
	"errors"
	"fmt"

	"github.com/Duke1616/etask/sdk/executor/internal/execution"
	"github.com/gotomicro/ego/server"
)

// Name 返回 Executor 服务名称。
func (e *Executor) Name() string {
	return e.config.Server.ServiceName
}

// PackageName 返回 Executor SDK 组件名称。
func (e *Executor) PackageName() string {
	return "sdk.executor"
}

// Init 完成 ego Server 生命周期的初始化阶段。
func (e *Executor) Init() error {
	if e.server == nil {
		return fmt.Errorf("executor 组件尚未初始化")
	}
	return e.server.Init()
}

// Start 启动 Executor gRPC Server。
func (e *Executor) Start() error {
	if e.server == nil {
		return fmt.Errorf("executor 组件尚未初始化")
	}
	return e.server.Start()
}

// Stop 停止 PULL 循环和 Executor gRPC Server。
func (e *Executor) Stop() error {
	e.stopPullLoop()
	e.stopAcceptingExecutions()
	e.executions.CancelAll(execution.ErrInterrupted)
	var err error
	if e.server == nil {
		err = nil
	} else {
		err = e.server.Stop()
	}
	return errors.Join(err, e.closeConnection())
}

// GracefulStop 停止 PULL 循环并优雅关闭 Executor gRPC Server。
func (e *Executor) GracefulStop(ctx context.Context) error {
	e.stopPullLoop()
	e.stopAcceptingExecutions()
	var err error
	if e.server == nil {
		err = nil
	} else {
		err = e.server.GracefulStop(ctx)
	}
	if waitErr := e.waitExecutions(ctx); waitErr != nil {
		e.executions.CancelAll(execution.ErrInterrupted)
		err = errors.Join(err, waitErr)
	}
	return errors.Join(err, e.closeConnection())
}

// Info 返回 Executor 服务注册信息。
func (e *Executor) Info() *server.ServiceInfo {
	if e.server == nil {
		return nil
	}
	return e.server.Info()
}

func (e *Executor) stopPullLoop() {
	e.initMu.Lock()
	defer e.initMu.Unlock()
	if e.pullCancel != nil {
		e.pullCancel()
		e.pullCancel = nil
	}
}

func (e *Executor) stopAcceptingExecutions() {
	e.runMu.Lock()
	e.stopping = true
	e.runMu.Unlock()
}

func (e *Executor) waitExecutions(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		e.runWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *Executor) closeConnection() error {
	e.initMu.Lock()
	defer e.initMu.Unlock()
	if e.schedulerConn == nil {
		return nil
	}
	err := e.schedulerConn.Close()
	e.schedulerConn = nil
	return err
}

var _ server.Server = (*Executor)(nil)
