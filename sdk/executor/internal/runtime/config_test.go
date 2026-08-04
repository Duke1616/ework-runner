package runtime

// Config 测试覆盖默认值、规范化和非法配置。

import (
	"context"
	"testing"

	grpcpkg "github.com/Duke1616/etask/pkg/grpc"
	"github.com/Duke1616/etask/pkg/grpc/registry"
	"github.com/Duke1616/etask/sdk/executor/internal/task"
	"github.com/stretchr/testify/require"
)

func TestNormalizeConfig(t *testing.T) {
	testCases := []struct {
		name      string
		config    Config
		registry  registry.Registry
		wantMode  string
		wantLevel string
		wantError string
	}{
		{
			name: "补全默认模式和隔离级别", registry: registryStub{},
			config: Config{Server: validServerConfig()}, wantMode: ModePush, wantLevel: IsolationShared,
		},
		{
			name: "规范化配置大小写", registry: registryStub{},
			config:   Config{Mode: " pull ", IsolationLevel: " dedicated ", Server: validServerConfig()},
			wantMode: ModePull, wantLevel: IsolationDedicated,
		},
		{name: "拒绝非法模式", registry: registryStub{}, config: Config{Mode: "unknown", Server: validServerConfig()}, wantError: "执行模式非法"},
		{name: "拒绝空注册中心", config: Config{Server: validServerConfig()}, wantError: "注册中心不能为空"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config, err := normalizeConfig(tc.config, tc.registry)
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantMode, config.Mode)
			require.Equal(t, tc.wantLevel, config.IsolationLevel)
		})
	}
}

func TestExecutorRejectsInvalidOrLateHandlerRegistration(t *testing.T) {
	executor, err := NewExecutor(Config{Server: validServerConfig()}, registryStub{})
	require.NoError(t, err)
	require.Error(t, executor.RegisterHandlers(nil))
	require.Error(t, executor.registrationErr)

	valid, err := NewExecutor(Config{Server: validServerConfig()}, registryStub{})
	require.NoError(t, err)
	require.NoError(t, valid.RegisterHandlers(runtimeHandlerStub{name: "shell"}))
	valid.initialized = true
	require.ErrorContains(t, valid.RegisterHandlers(runtimeHandlerStub{name: "python"}), "已初始化")
}

type runtimeHandlerStub struct{ name string }

func (h runtimeHandlerStub) Name() string             { return h.name }
func (h runtimeHandlerStub) Desc() string             { return h.name }
func (runtimeHandlerStub) Metadata() []task.Parameter { return nil }
func (runtimeHandlerStub) Run(*task.Context) error    { return nil }

func validServerConfig() grpcpkg.ServerConfig {
	return grpcpkg.ServerConfig{ServiceId: "node-1", ServiceName: "executor", ListenAddr: "127.0.0.1:0"}
}

type registryStub struct{}

func (registryStub) Register(context.Context, registry.ServiceInstance) error   { return nil }
func (registryStub) UnRegister(context.Context, registry.ServiceInstance) error { return nil }
func (registryStub) ListServices(context.Context, string) ([]registry.ServiceInstance, error) {
	return nil, nil
}
func (registryStub) Subscribe(string) <-chan registry.Event { return nil }
func (registryStub) Close() error                           { return nil }

var _ registry.Registry = registryStub{}
