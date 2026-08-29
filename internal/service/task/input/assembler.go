package input

import (
	"context"
	"fmt"
	"maps"

	"github.com/Duke1616/etask/internal/domain"
	runnerSvc "github.com/Duke1616/etask/internal/service/runner"
	taskbinding "github.com/Duke1616/etask/internal/service/task/binding"
	"github.com/samber/lo"
)

// AssembleResult 保存组装后的任务快照和结构化变量快照。
type AssembleResult struct {
	Task      domain.Task
	Variables *domain.ExecutionVariableSet
}

// ExecutionInputAssembler 负责集中处理参数、变量和绑定的优先级。
type ExecutionInputAssembler interface {
	// Assemble 生成本次执行使用的最终任务输入，不修改调用方传入的任务。
	Assemble(ctx context.Context, task domain.Task) (AssembleResult, error)
}

// executionInputAssembler 负责组装一次执行所需的最终输入。
type executionInputAssembler struct {
	runnerSvc runnerSvc.Service
	resolvers *taskbinding.Registry
}

// NewExecutionInputAssembler 创建默认执行输入组装器。
func NewExecutionInputAssembler(runnerService runnerSvc.Service,
	resolvers *taskbinding.Registry) ExecutionInputAssembler {
	return &executionInputAssembler{
		runnerSvc: runnerService,
		resolvers: resolvers,
	}
}

// Assemble 按任务类型组装 Runner、任务配置和绑定解析产生的最终输入。
func (a *executionInputAssembler) Assemble(ctx context.Context, task domain.Task) (AssembleResult, error) {
	if task.RunnerID != 0 {
		return a.assembleRunner(ctx, task)
	}
	return a.assembleTask(ctx, task)
}

func (a *executionInputAssembler) assembleRunner(ctx context.Context, task domain.Task) (AssembleResult, error) {
	if task.GrpcConfig == nil {
		return AssembleResult{}, fmt.Errorf("引用执行单元的任务缺少 gRPC 配置")
	}
	spec, err := a.runnerSvc.FindForExecution(ctx, task.RunnerID)
	if err != nil {
		return AssembleResult{}, fmt.Errorf("查询执行单元失败: %w", err)
	}
	if spec.Runner.Action != domain.RunnerActionRegistered {
		return AssembleResult{}, fmt.Errorf("执行单元未启用")
	}
	params, err := (ParameterMerger{}).Merge(ParameterMergeInput{
		RunnerDefaults:   spec.Runner.ParameterDefaults,
		TaskParams:       task.GrpcConfig.Params,
		RuntimeOverrides: task.PendingParamOverrides,
	})
	if err != nil {
		return AssembleResult{}, err
	}
	config := *task.GrpcConfig
	config.Params = maps.Clone(params)
	task.GrpcConfig = &config
	task.Metadata = maps.Clone(task.Metadata)
	task.PendingParamOverrides = maps.Clone(task.PendingParamOverrides)

	layers := lo.Filter([]VariableLayer{
		{Source: VariableSourceRunner, Items: spec.Variables},
		{Source: VariableSourceTask, Items: config.Variables},
	}, func(layer VariableLayer, _ int) bool {
		return len(layer.Items) > 0
	})
	if len(layers) == 0 {
		return AssembleResult{Task: task}, nil
	}
	variables, err := (VariableMerger{}).Merge(layers...)
	if err != nil {
		return AssembleResult{}, fmt.Errorf("合并执行单元变量失败: %w", err)
	}
	return AssembleResult{Task: task, Variables: &domain.ExecutionVariableSet{Items: variables}}, nil
}

func (a *executionInputAssembler) assembleTask(ctx context.Context, task domain.Task) (AssembleResult, error) {
	if task.GrpcConfig == nil {
		return AssembleResult{Task: task}, nil
	}
	resolved, err := a.resolvers.Resolve(ctx, task.GrpcConfig.HandlerName,
		task.GrpcConfig.Params, task.Metadata)
	if err != nil {
		return AssembleResult{}, err
	}
	params, err := (ParameterMerger{}).Merge(ParameterMergeInput{
		TaskParams:       task.GrpcConfig.Params,
		BindingParams:    resolved.Parameters,
		RuntimeOverrides: task.PendingParamOverrides,
	})
	if err != nil {
		return AssembleResult{}, err
	}
	config := *task.GrpcConfig
	config.Params = maps.Clone(params)
	task.GrpcConfig = &config
	task.Metadata = maps.Clone(task.Metadata)
	task.PendingParamOverrides = maps.Clone(task.PendingParamOverrides)

	layers := lo.Filter([]VariableLayer{
		{Source: VariableSourceTask, Items: config.Variables},
		{Source: VariableSourceBinding, Items: resolved.Variables},
	}, func(layer VariableLayer, _ int) bool {
		return len(layer.Items) > 0
	})
	if len(layers) == 0 {
		return AssembleResult{Task: task}, nil
	}
	variables, err := (VariableMerger{}).Merge(layers...)
	if err != nil {
		return AssembleResult{}, fmt.Errorf("合并任务变量失败: %w", err)
	}
	return AssembleResult{Task: task, Variables: &domain.ExecutionVariableSet{Items: variables}}, nil
}

var _ ExecutionInputAssembler = (*executionInputAssembler)(nil)
