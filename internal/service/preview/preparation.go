package preview

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Duke1616/etask/internal/domain"
	programSvc "github.com/Duke1616/etask/internal/service/program"
	runnerSvc "github.com/Duke1616/etask/internal/service/runner"
)

type prepareResult struct {
	runner          domain.Runner
	program         *domain.Program
	sourceProjectID int64
	args            string
	params          map[string]string
	timeout         int64
	variables       []domain.RunnerVariable
}

func (s *service) prepare(ctx context.Context, command RunCommand) (prepareResult, error) {
	if err := validateCommand(command); err != nil {
		return prepareResult{}, err
	}
	spec, err := s.resolveRunner(ctx, command.RunnerID)
	if err != nil {
		return prepareResult{}, err
	}
	runner := spec.Runner
	params, err := runnerSvc.MergeParameterDefaults(runner.ParameterDefaults, nil)
	if err != nil {
		return prepareResult{}, err
	}
	args, err := normalizeArgs(command.Args, runner.ParameterDefaults)
	if err != nil {
		return prepareResult{}, err
	}
	params["args"] = args
	timeout, err := normalizeTimeout(command.MaxExecutionSeconds)
	if err != nil {
		return prepareResult{}, err
	}
	variables, err := mergeVariables(spec.Variables, command.Variables)
	if err != nil {
		return prepareResult{}, err
	}
	resolution, err := s.resolveProgram(ctx, runner)
	if err != nil {
		return prepareResult{}, fmt.Errorf("解析试运行程序失败: %w", err)
	}
	if resolution.Program == nil {
		return prepareResult{}, fmt.Errorf("试运行程序不能为空")
	}
	return prepareResult{
		runner: runner, program: resolution.Program, sourceProjectID: resolution.SourceProjectID,
		args: args, params: params, timeout: timeout, variables: variables,
	}, nil
}

func validateCommand(command RunCommand) error {
	if command.RunnerID <= 0 {
		return fmt.Errorf("执行单元 ID 必须大于 0")
	}
	return nil
}

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

func normalizeArgs(raw string, defaults map[string]json.RawMessage) (string, error) {
	args := strings.TrimSpace(raw)
	if args == "" {
		if value, exists := defaults["args"]; exists {
			var err error
			args, err = runnerSvc.ParameterDefaultValue(value)
			if err != nil {
				return "", fmt.Errorf("Runner 默认 args 非法: %w", err)
			}
			args = strings.TrimSpace(args)
		}
		if args == "" {
			return "{}", nil
		}
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

// mergeVariables 保留默认变量顺序；临时变量覆盖同名值，新变量追加到末尾。
func mergeVariables(defaults, overrides []domain.RunnerVariable) ([]domain.RunnerVariable, error) {
	values := make(map[string]domain.RunnerVariable, len(defaults)+len(overrides))
	keys := make([]string, 0, len(defaults)+len(overrides))
	for _, variable := range defaults {
		values[variable.Key] = variable
		keys = append(keys, variable.Key)
	}
	for _, variable := range overrides {
		variable.Key = strings.TrimSpace(variable.Key)
		if variable.Key == "" {
			return nil, fmt.Errorf("临时变量名称不能为空")
		}
		if _, exists := values[variable.Key]; !exists {
			keys = append(keys, variable.Key)
		}
		values[variable.Key] = variable
	}
	result := make([]domain.RunnerVariable, 0, len(keys))
	for _, key := range keys {
		result = append(result, values[key])
	}
	return result, nil
}

func (s *service) buildDraft(prepared prepareResult) domain.TaskExecution {
	params := make(map[string]string, len(prepared.params))
	for key, value := range prepared.params {
		params[key] = value
	}
	variables := append([]domain.RunnerVariable(nil), prepared.variables...)
	return domain.TaskExecution{
		Status: domain.TaskExecutionStatusPrepare, StartTime: time.Now().UnixMilli(),
		Task: domain.Task{
			RunnerID:            prepared.runner.ID,
			Name:                "试运行: " + prepared.runner.Name,
			MaxExecutionSeconds: prepared.timeout,
			RetryConfig:         &domain.RetryConfig{MaxRetries: 0},
			GrpcConfig: &domain.GrpcConfig{
				ServiceName: prepared.runner.Target, HandlerName: prepared.runner.Handler,
				Params: params,
			},
		},
		Program:   prepared.program,
		Variables: &domain.ExecutionVariableSet{Items: variables},
	}
}
