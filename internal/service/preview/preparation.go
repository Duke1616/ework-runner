package preview

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Duke1616/etask/internal/domain"
	programSvc "github.com/Duke1616/etask/internal/service/program"
)

type prepareResult struct {
	runner          domain.Runner
	program         *domain.Program
	sourceProjectID int64
	args            string
	timeout         int64
	variables       []previewVariable
}

func (s *service) prepare(ctx context.Context, command RunCommand) (prepareResult, error) {
	if err := validateCommand(command); err != nil {
		return prepareResult{}, err
	}
	runner, err := s.resolveRunner(ctx, command.RunnerID)
	if err != nil {
		return prepareResult{}, err
	}
	args, err := normalizeArgs(command.Args)
	if err != nil {
		return prepareResult{}, err
	}
	timeout, err := normalizeTimeout(command.MaxExecutionSeconds)
	if err != nil {
		return prepareResult{}, err
	}
	variables, err := mergeVariables(runner.Variables, command.Variables)
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
		args: args, timeout: timeout, variables: variables,
	}, nil
}

func validateCommand(command RunCommand) error {
	if command.RunnerID <= 0 {
		return fmt.Errorf("执行单元 ID 必须大于 0")
	}
	return nil
}

func (s *service) resolveRunner(ctx context.Context, id int64) (domain.Runner, error) {
	runner, err := s.runnerSvc.FindForExecution(ctx, id)
	if err != nil {
		return domain.Runner{}, fmt.Errorf("查询执行单元失败: %w", err)
	}
	if runner.CodebookID <= 0 {
		return domain.Runner{}, fmt.Errorf("执行单元未绑定程序来源")
	}
	if !runner.ProgramKind.Valid() {
		return domain.Runner{}, fmt.Errorf("执行单元程序类型非法: %s", runner.ProgramKind)
	}
	if !runner.Kind.IsValid() {
		return domain.Runner{}, fmt.Errorf("执行单元类型非法: %s", runner.Kind)
	}
	if strings.TrimSpace(runner.Handler) == "" {
		return domain.Runner{}, fmt.Errorf("执行单元未配置 Handler")
	}
	if runner.Action != domain.RunnerActionRegistered {
		return domain.Runner{}, fmt.Errorf("当前执行单元未启用")
	}
	return runner, nil
}

func (s *service) resolveProgram(ctx context.Context, runner domain.Runner) (programSvc.Resolution, error) {
	spec, err := programSvc.SpecFromRunnerBinding(runner.CodebookID, runner.ProgramKind)
	if err != nil {
		return programSvc.Resolution{}, err
	}
	return s.programSvc.Resolve(ctx, spec)
}

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

// mergeVariables 保留默认变量顺序；临时变量覆盖同名值，新变量追加到末尾。
func mergeVariables(defaults, overrides []domain.RunnerVariable) ([]previewVariable, error) {
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
	result := make([]previewVariable, 0, len(keys))
	for _, key := range keys {
		variable := values[key]
		result = append(result, previewVariable{Key: key, Value: variable.Value, Secret: variable.Secret})
	}
	return result, nil
}

func (s *service) buildDraft(prepared prepareResult, variablesJSON []byte) domain.TaskExecution {
	return domain.TaskExecution{
		Status: domain.TaskExecutionStatusPrepare, StartTime: time.Now().UnixMilli(),
		Task: domain.Task{
			Name:                "试运行: " + prepared.runner.Name,
			MaxExecutionSeconds: prepared.timeout,
			RetryConfig:         &domain.RetryConfig{MaxRetries: 0},
			GrpcConfig: &domain.GrpcConfig{
				ServiceName: prepared.runner.Target, HandlerName: prepared.runner.Handler,
				Params: map[string]string{
					"args": prepared.args, "variables": string(variablesJSON),
				},
			},
		},
		Program: prepared.program,
	}
}

type previewVariable struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Secret bool   `json:"secret"`
}
