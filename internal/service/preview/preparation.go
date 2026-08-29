package preview

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/Duke1616/etask/internal/domain"
	programSvc "github.com/Duke1616/etask/internal/service/program"
	runnerSvc "github.com/Duke1616/etask/internal/service/runner"
	taskinput "github.com/Duke1616/etask/internal/service/task/input"
)

// prepareResult 聚合试运行任务所需的上下文与快照数据。
type prepareResult struct {
	runner          domain.Runner
	program         *domain.Program
	sourceProjectID int64
	args            string
	params          map[string]string
	timeout         int64
	variables       []domain.RunnerVariable
}

// prepare 校验命令合法性，合并默认参数与临时入参，并解析不可变程序快照。
func (s *service) prepare(ctx context.Context, command RunCommand) (prepareResult, error) {
	if err := validateCommand(command); err != nil {
		return prepareResult{}, err
	}
	spec, err := s.resolveRunner(ctx, command.RunnerID)
	if err != nil {
		return prepareResult{}, err
	}
	runner := spec.Runner

	// 合并执行单元默认参数与用户本次输入的参数
	params, err := (taskinput.ParameterMerger{}).Merge(taskinput.ParameterMergeInput{
		RunnerDefaults: runner.ParameterDefaults,
		TaskParams:     command.Params,
	})
	if err != nil {
		return prepareResult{}, err
	}

	// 校验并归一化 args 参数（若为空自动兜底为 "{}"）
	args, err := normalizeArgs(params[runnerSvc.ParameterKeyArgs])
	if err != nil {
		return prepareResult{}, err
	}
	params[runnerSvc.ParameterKeyArgs] = args

	timeout, err := normalizeTimeout(command.MaxExecutionSeconds)
	if err != nil {
		return prepareResult{}, err
	}

	// 合并 Runner 静态变量与试运行临时注入的变量
	variables, err := (taskinput.VariableMerger{}).Merge(
		taskinput.VariableLayer{Source: taskinput.VariableSourceRunner, Items: spec.Variables},
		taskinput.VariableLayer{Source: taskinput.VariableSourceTask, Items: command.Variables},
	)
	if err != nil {
		return prepareResult{}, err
	}

	// 解析关联程序并固定不可变快照
	resolution, err := s.resolveProgram(ctx, runner)
	if err != nil {
		return prepareResult{}, fmt.Errorf("解析试运行程序失败: %w", err)
	}
	if resolution.Program == nil {
		return prepareResult{}, fmt.Errorf("试运行程序不能为空")
	}

	return prepareResult{
		runner:          runner,
		program:         resolution.Program,
		sourceProjectID: resolution.SourceProjectID,
		args:            args,
		params:          params,
		timeout:         timeout,
		variables:       variables,
	}, nil
}

func validateCommand(command RunCommand) error {
	if command.RunnerID <= 0 {
		return fmt.Errorf("执行单元 ID 必须大于 0")
	}
	return nil
}

// resolveRunner 查询并校验 Runner 处于已启用且配置完备状态。
func (s *service) resolveRunner(ctx context.Context, id int64) (domain.RunnerExecutionSpec, error) {
	spec, err := s.runnerSvc.FindForExecution(ctx, id)
	if err != nil {
		return domain.RunnerExecutionSpec{}, fmt.Errorf("查询执行单元失败: %w", err)
	}
	runner := spec.Runner
	if runner.CodebookID <= 0 {
		return domain.RunnerExecutionSpec{}, fmt.Errorf("执行单元未绑定程序来源")
	}
	if !runner.ProgramKind.Valid() {
		return domain.RunnerExecutionSpec{}, fmt.Errorf("执行单元程序类型非法: %s", runner.ProgramKind)
	}
	if !runner.Kind.IsValid() {
		return domain.RunnerExecutionSpec{}, fmt.Errorf("执行单元类型非法: %s", runner.Kind)
	}
	if strings.TrimSpace(runner.Handler) == "" {
		return domain.RunnerExecutionSpec{}, fmt.Errorf("执行单元未配置 Handler")
	}
	if runner.Action != domain.RunnerActionRegistered {
		return domain.RunnerExecutionSpec{}, fmt.Errorf("当前执行单元未启用")
	}
	return spec, nil
}

func (s *service) resolveProgram(ctx context.Context, runner domain.Runner) (programSvc.Resolution, error) {
	spec, err := programSvc.SpecFromRunnerBinding(runner.CodebookID, runner.ProgramKind)
	if err != nil {
		return programSvc.Resolution{}, err
	}
	return s.programSvc.Resolve(ctx, spec)
}

// normalizeArgs 校验并归一化执行参数，空参数默认兜底为空 JSON 对象 "{}"。
func normalizeArgs(raw string) (string, error) {
	args := strings.TrimSpace(raw)
	if args == "" {
		return "{}", nil
	}
	if !json.Valid([]byte(args)) {
		return "", fmt.Errorf("试运行参数必须是合法 JSON")
	}
	return args, nil
}

func normalizeTimeout(seconds int64) (int64, error) {
	if seconds == 0 {
		seconds = defaultTimeoutSeconds
	}
	if seconds < 1 || seconds > maxTimeoutSeconds {
		return 0, fmt.Errorf("试运行超时必须在 1 到 %d 秒之间", maxTimeoutSeconds)
	}
	return seconds, nil
}

// buildDraft 根据预检结果组装未持久化的试运行 TaskExecution 草稿。
func (s *service) buildDraft(prepared prepareResult) domain.TaskExecution {
	return domain.TaskExecution{
		Status:    domain.TaskExecutionStatusPrepare,
		StartTime: time.Now().UnixMilli(),
		Task: domain.Task{
			RunnerID:            prepared.runner.ID,
			Name:                "试运行: " + prepared.runner.Name,
			MaxExecutionSeconds: prepared.timeout,
			RetryConfig:         &domain.RetryConfig{MaxRetries: 0},
			GrpcConfig: &domain.GrpcConfig{
				ServiceName: prepared.runner.Target,
				HandlerName: prepared.runner.Handler,
				Params:      maps.Clone(prepared.params),
			},
		},
		Program:   prepared.program,
		Variables: &domain.ExecutionVariableSet{Items: slices.Clone(prepared.variables)},
	}
}
