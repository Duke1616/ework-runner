package node_test

import (
	"context"
	"testing"

	grpcpkg "github.com/Duke1616/etask/pkg/grpc"
	"github.com/Duke1616/etask/pkg/grpc/registry"
	"github.com/Duke1616/etask/sdk/executor"
	"github.com/Duke1616/etask/sdk/executor/node"
	"github.com/gotomicro/ego/server"
	"github.com/stretchr/testify/require"
)

func TestNewBuildsReadyEgoServer(t *testing.T) {
	exec, err := node.New(node.Config{
		Mode: node.ModePush,
		Server: grpcpkg.ServerConfig{
			ServiceId: "test-node", ServiceName: "test-executor", ListenAddr: "127.0.0.1:0",
		},
		Client: grpcpkg.ClientConfig{Name: "scheduler"},
	}, registryStub{}, handlerStub{})
	require.NoError(t, err)
	require.NotNil(t, exec.Info())
	require.NoError(t, exec.Stop())

	var _ server.Server = exec
}

func TestNewReturnsHandlerRegistrationErrors(t *testing.T) {
	_, err := node.New(node.Config{
		Server: grpcpkg.ServerConfig{
			ServiceId: "test-node", ServiceName: "test-executor", ListenAddr: "127.0.0.1:0",
		},
	}, registryStub{}, handlerStub{}, handlerStub{})
	require.ErrorContains(t, err, "simple")
}

type handlerStub struct{}

func (handlerStub) Name() string                   { return "simple" }
func (handlerStub) Desc() string                   { return "simple handler" }
func (handlerStub) Metadata() []executor.Parameter { return nil }
func (handlerStub) Run(*executor.Context) error    { return nil }

type registryStub struct{}

func (registryStub) Register(context.Context, registry.ServiceInstance) error   { return nil }
func (registryStub) UnRegister(context.Context, registry.ServiceInstance) error { return nil }
func (registryStub) ListServices(context.Context, string) ([]registry.ServiceInstance, error) {
	return nil, nil
}
func (registryStub) Subscribe(string) <-chan registry.Event { return make(chan registry.Event) }
func (registryStub) Close() error                           { return nil }
