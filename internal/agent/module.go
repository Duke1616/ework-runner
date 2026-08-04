package agent

import (
	"context"
	"errors"

	"github.com/Duke1616/etask/internal/agent/service"
	"github.com/Duke1616/etask/sdk/executor/artifact"
	"github.com/gotomicro/ego/core/constant"
	"github.com/gotomicro/ego/server"
	"google.golang.org/grpc"
)

type Consumer interface {
	// Start 启动消息消费循环。
	Start(ctx context.Context) error
	// Stop 停止消费并释放资源。
	Stop(ctx context.Context) error
}

type Module struct {
	Svc        service.Service
	C          Consumer
	ctx        context.Context
	cancel     context.CancelFunc
	connection *grpc.ClientConn
	artifacts  artifact.Preparer
}

func (m *Module) GetConsumer() Consumer {
	return m.C
}

// --- 实现 server.Server 接口 ---

func (m *Module) Name() string {
	return "agent_consumer"
}

func (m *Module) PackageName() string {
	return "internal.agent"
}

func (m *Module) Init() error {
	if m.artifacts != nil {
		return m.artifacts.Prune()
	}
	return nil
}

func (m *Module) Start() error {
	m.ctx, m.cancel = context.WithCancel(context.Background())
	return m.C.Start(m.ctx)
}

func (m *Module) Stop() error {
	if m.cancel != nil {
		m.cancel()
	}
	err := m.C.Stop(context.Background())
	if m.connection != nil {
		err = errors.Join(err, m.connection.Close())
	}
	return err
}

func (m *Module) GracefulStop(ctx context.Context) error {
	if m.cancel != nil {
		m.cancel()
	}
	err := m.C.Stop(ctx)
	if m.connection != nil {
		err = errors.Join(err, m.connection.Close())
	}
	return err
}

func (m *Module) Info() *server.ServiceInfo {
	info := server.ApplyOptions(
		server.WithName(m.Name()),
		server.WithKind(constant.ServiceConsumer),
	)
	return &info
}
