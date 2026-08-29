package input

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/pkg/variable"
	runnerSvc "github.com/Duke1616/etask/internal/service/runner"
	runnermocks "github.com/Duke1616/etask/internal/service/runner/mocks"
	taskbinding "github.com/Duke1616/etask/internal/service/task/binding"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestExecutionInputAssembler_Assemble(t *testing.T) {
	testCases := []struct {
		name       string
		setupMocks func(ctrl *gomock.Controller) (runnerSvc.Service, *taskbinding.Registry)
		inputTask  domain.Task
		verify     func(t *testing.T, result AssembleResult, err error)
	}{
		{
			name: "原生任务_参数与变量按优先级明确解析并保留绑定Secret",
			setupMocks: func(ctrl *gomock.Controller) (runnerSvc.Service, *taskbinding.Registry) {
				resolvers := taskbinding.NewRegistry().Register("binding", taskbinding.ResolverFunc(
					func(_ context.Context, req taskbinding.ResolveRequest) (taskbinding.ResolveResult, error) {
						return taskbinding.ResolveResult{
							Parameters: map[string]string{req.ParamKey: "binding"},
							Variables: []variable.Item{
								{Key: "TOKEN", Value: "binding", Secret: true},
							},
						}, nil
					},
				))
				return nil, resolvers
			},
			inputTask: domain.Task{
				GrpcConfig: &domain.GrpcConfig{
					HandlerName: "shell",
					Params:      map[string]string{"region": "task", "binding_param": "raw"},
					Variables:   []variable.Item{{Key: "TOKEN", Value: "task", Secret: false}},
				},
				Metadata:              map[string]string{"region": "binding"},
				PendingParamOverrides: map[string]string{"region": "runtime"},
			},
			verify: func(t *testing.T, result AssembleResult, err error) {
				require.NoError(t, err)
				require.Equal(t, "runtime", result.Task.GrpcConfig.Params["region"])
				require.Equal(t, "raw", result.Task.GrpcConfig.Params["binding_param"])
				require.Equal(t, []variable.Item{{Key: "TOKEN", Value: "binding", Secret: true}}, result.Variables.Items)
			},
		},
		{
			name: "Runner任务_变量层叠合并与Secret属性保留",
			setupMocks: func(ctrl *gomock.Controller) (runnerSvc.Service, *taskbinding.Registry) {
				mockRunner := runnermocks.NewMockService(ctrl)
				mockRunner.EXPECT().FindForExecution(gomock.Any(), int64(101)).Return(domain.RunnerExecutionSpec{
					Runner: domain.Runner{
						ID:     101,
						Action: domain.RunnerActionRegistered,
						ParameterDefaults: map[string]json.RawMessage{
							"env": json.RawMessage(`"prod"`),
						},
					},
					Variables: []domain.RunnerVariable{
						{Key: "API_KEY", Value: "runner-secret", Secret: true},
						{Key: "CLUSTER", Value: "default-cluster", Secret: false},
					},
				}, nil)
				return mockRunner, nil
			},
			inputTask: domain.Task{
				RunnerID: 101,
				GrpcConfig: &domain.GrpcConfig{
					HandlerName: "ansible",
					Params:      map[string]string{"env": "staging"},
					Variables: []variable.Item{
						{Key: "API_KEY", Value: "task-overridden", Secret: false}, // 显式 false，继承底层 true
						{Key: "EXTRA_FLAG", Value: "enabled", Secret: false},
					},
				},
				PendingParamOverrides: map[string]string{"env": "test"},
			},
			verify: func(t *testing.T, result AssembleResult, err error) {
				require.NoError(t, err)
				require.Equal(t, "test", result.Task.GrpcConfig.Params["env"])
				require.NotNil(t, result.Variables)
				require.Equal(t, []variable.Item{
					{Key: "API_KEY", Value: "task-overridden", Secret: true},
					{Key: "CLUSTER", Value: "default-cluster", Secret: false},
					{Key: "EXTRA_FLAG", Value: "enabled", Secret: false},
				}, result.Variables.Items)
			},
		},
		{
			name: "Runner任务_引用执行单元缺少GrpcConfig报错",
			setupMocks: func(ctrl *gomock.Controller) (runnerSvc.Service, *taskbinding.Registry) {
				return nil, nil
			},
			inputTask: domain.Task{
				RunnerID:   102,
				GrpcConfig: nil,
			},
			verify: func(t *testing.T, result AssembleResult, err error) {
				require.Error(t, err)
				require.ErrorContains(t, err, "引用执行单元的任务缺少 gRPC 配置")
			},
		},
		{
			name: "Runner任务_执行单元未启用报错",
			setupMocks: func(ctrl *gomock.Controller) (runnerSvc.Service, *taskbinding.Registry) {
				mockRunner := runnermocks.NewMockService(ctrl)
				mockRunner.EXPECT().FindForExecution(gomock.Any(), int64(103)).Return(domain.RunnerExecutionSpec{
					Runner: domain.Runner{
						ID:     103,
						Action: domain.RunnerActionUnregistered, // 未启用
					},
				}, nil)
				return mockRunner, nil
			},
			inputTask: domain.Task{
				RunnerID: 103,
				GrpcConfig: &domain.GrpcConfig{
					HandlerName: "shell",
				},
			},
			verify: func(t *testing.T, result AssembleResult, err error) {
				require.Error(t, err)
				require.ErrorContains(t, err, "执行单元未启用")
			},
		},
		{
			name: "Runner任务_查询执行单元失败透传错误",
			setupMocks: func(ctrl *gomock.Controller) (runnerSvc.Service, *taskbinding.Registry) {
				mockRunner := runnermocks.NewMockService(ctrl)
				mockRunner.EXPECT().FindForExecution(gomock.Any(), int64(104)).Return(
					domain.RunnerExecutionSpec{}, errors.New("db error"),
				)
				return mockRunner, nil
			},
			inputTask: domain.Task{
				RunnerID: 104,
				GrpcConfig: &domain.GrpcConfig{
					HandlerName: "shell",
				},
			},
			verify: func(t *testing.T, result AssembleResult, err error) {
				require.Error(t, err)
				require.ErrorContains(t, err, "查询执行单元失败")
			},
		},
		{
			name: "不可变深拷贝_修改原始任务Map不影响输出快照",
			setupMocks: func(ctrl *gomock.Controller) (runnerSvc.Service, *taskbinding.Registry) {
				return nil, nil
			},
			inputTask: func() domain.Task {
				return domain.Task{
					GrpcConfig: &domain.GrpcConfig{
						HandlerName: "test",
						Params:      map[string]string{"key": "original"},
					},
					Metadata:              map[string]string{"meta": "data"},
					PendingParamOverrides: map[string]string{"runtime": "val"},
				}
			}(),
			verify: func(t *testing.T, result AssembleResult, err error) {
				require.NoError(t, err)
				// 快照正确生成
				require.Equal(t, "original", result.Task.GrpcConfig.Params["key"])
				require.Equal(t, "val", result.Task.PendingParamOverrides["runtime"])
				require.Equal(t, "data", result.Task.Metadata["meta"])
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			var runnerService runnerSvc.Service
			var resolvers *taskbinding.Registry
			if tc.setupMocks != nil {
				runnerService, resolvers = tc.setupMocks(ctrl)
			}
			assembler := NewExecutionInputAssembler(runnerService, resolvers)
			result, err := assembler.Assemble(context.Background(), tc.inputTask)
			tc.verify(t, result, err)
		})
	}
}

func TestExecutionInputAssembler_DeepImmutabilityDirectMutation(t *testing.T) {
	assembler := NewExecutionInputAssembler(nil, nil)

	origParams := map[string]string{"key": "original"}
	origOverrides := map[string]string{"runtime": "val"}
	origMetadata := map[string]string{"meta": "data"}

	task := domain.Task{
		GrpcConfig: &domain.GrpcConfig{
			HandlerName: "test",
			Params:      origParams,
		},
		Metadata:              origMetadata,
		PendingParamOverrides: origOverrides,
	}

	result, err := assembler.Assemble(context.Background(), task)
	require.NoError(t, err)

	// 修改原始任务中的 map
	origParams["key"] = "mutated"
	origOverrides["runtime"] = "mutated"
	origMetadata["meta"] = "mutated"

	// 组装结果中的快照严格保持不变
	require.Equal(t, "original", result.Task.GrpcConfig.Params["key"])
	require.Equal(t, "val", result.Task.PendingParamOverrides["runtime"])
	require.Equal(t, "data", result.Task.Metadata["meta"])
}
