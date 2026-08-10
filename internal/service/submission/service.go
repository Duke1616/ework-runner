// Package submission 负责外部系统正式执行请求的幂等创建和派发。
package submission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository"
	"github.com/Duke1616/etask/internal/service/dispatcher"
	"github.com/Duke1616/etask/internal/service/invoker"
	programSvc "github.com/Duke1616/etask/internal/service/program"
	runnerSvc "github.com/Duke1616/etask/internal/service/runner"
	taskSvc "github.com/Duke1616/etask/internal/service/task"
	terminationSvc "github.com/Duke1616/etask/internal/service/termination"
	"github.com/gotomicro/ego/core/elog"
)

const defaultTimeoutSeconds int64 = 300

var (
	// ErrInvalidCommand 表示调用参数不满足外部执行协议。
	ErrInvalidCommand = errors.New("工作流执行请求参数非法")
	// ErrRejected 表示当前 Runner 或路由不允许执行该请求。
	ErrRejected = errors.New("工作流执行请求被拒绝")
)

//go:generate go tool mockgen -source=./service.go -package=submissionmocks -destination=./mocks/service.mock.go -typed

// RunRunnerCommand 描述外部工作流提交的一次幂等 Runner 执行。
type RunRunnerCommand struct {
	RequestID string
	RunnerID  int64
	Params    map[string]string
	Variables map[string]string
}

// TerminateExecutionCommand 描述外部工作流对一次 execution 的幂等终止请求。
type TerminateExecutionCommand struct {
	ExecutionID int64
	RequestID   string
	Reason      string
}

// RunResult 返回执行记录以及本次调用是否创建了新记录。
type RunResult struct {
	Execution domain.TaskExecution
	Created   bool
}

// Service 定义外部工作流提交 Runner 执行的应用能力。
type Service interface {
	// RunRunner 幂等创建执行记录，并在 PUSH 模式下异步派发。
	RunRunner(ctx context.Context, command RunRunnerCommand) (RunResult, error)
	// TerminateExecution 将工作流 execution 置为 CANCELLED 并投递实际取消信号。
	TerminateExecution(ctx context.Context, command TerminateExecutionCommand) error
}

type service struct {
	runners     runnerSvc.Service
	programs    programSvc.Service
	executions  taskSvc.ExecutionService
	routes      dispatcher.RoutePlanner
	invoker     invoker.Invoker
	termination terminationSvc.Service
	logger      *elog.Component
}

// NewService 创建外部执行提交服务。
func NewService(runners runnerSvc.Service, programs programSvc.Service,
	executions taskSvc.ExecutionService, routes dispatcher.RoutePlanner,
	executionInvoker invoker.Invoker, termination terminationSvc.Service) Service {
	return &service{
		runners: runners, programs: programs, executions: executions,
		routes: routes, invoker: executionInvoker, termination: termination,
		logger: elog.DefaultLogger.With(elog.FieldComponentName("service.submission")),
	}
}

func (s *service) RunRunner(ctx context.Context, command RunRunnerCommand) (RunResult, error) {
	if err := validateCommand(command); err != nil {
		return RunResult{}, fmt.Errorf("%w: %v", ErrInvalidCommand, err)
	}
	runner, err := s.runners.FindForExecution(ctx, command.RunnerID)
	if err != nil {
		return RunResult{}, fmt.Errorf("查询执行单元失败: %w", err)
	}
	if runner.Action != domain.RunnerActionRegistered {
		return RunResult{}, fmt.Errorf("%w: 执行单元未启用", ErrRejected)
	}
	program, err := s.resolveProgram(ctx, runner.CodebookID, runner.ProgramKind)
	if err != nil {
		return RunResult{}, fmt.Errorf("解析工作流程序失败: %w", err)
	}
	params, err := s.buildParams(runner, command)
	if err != nil {
		return RunResult{}, err
	}
	draft := domain.TaskExecution{
		RequestID: command.RequestID,
		Status:    domain.TaskExecutionStatusPrepare,
		StartTime: time.Now().UnixMilli(),
		Task: domain.Task{
			Name:                "工作流执行: " + runner.Name,
			MaxExecutionSeconds: defaultTimeoutSeconds,
			RetryConfig:         &domain.RetryConfig{MaxRetries: 0},
			GrpcConfig: &domain.GrpcConfig{
				ServiceName: runner.Target,
				HandlerName: runner.Handler,
				Params:      params,
			},
		},
		Program: program.Program,
	}

	route, err := s.routes.Plan(ctx, draft.Task)
	if err != nil {
		return RunResult{}, classifyRouteError(runner.Target, err)
	}
	if route.Execution.Transport != runner.Kind.Transport() {
		return RunResult{}, fmt.Errorf("%w: 执行单元类型 %s 与资源池传输通道 %s 不一致", ErrRejected,
			runner.Kind, route.Execution.Transport)
	}
	draft.Task = route.Task
	draft.Route = route.Execution
	if draft.Task.ExecMode.IsPull() {
		draft.Status = domain.TaskExecutionStatusWaitingPull
	}

	execution, created, err := s.executions.CreateWorkflow(ctx, draft, program.SourceProjectID)
	if err != nil {
		return RunResult{}, fmt.Errorf("创建工作流执行记录失败: %w", err)
	}
	execution, err = s.termination.Attach(ctx, execution)
	if err != nil {
		return RunResult{}, fmt.Errorf("绑定工作流取消意图失败: %w", err)
	}
	if created && !execution.Task.ExecMode.IsPull() && !execution.Status.IsCancelled() {
		go s.invoke(route.Context(context.WithoutCancel(ctx)), execution)
	}
	return RunResult{Execution: execution, Created: created}, nil
}

func (s *service) TerminateExecution(ctx context.Context,
	command TerminateExecutionCommand) error {
	err := s.termination.Request(ctx, terminationSvc.Request{
		ExecutionID: command.ExecutionID, RequestID: command.RequestID, Reason: command.Reason,
	})
	if err != nil {
		if errors.Is(err, terminationSvc.ErrInvalidCommand) {
			return fmt.Errorf("%w: %v", ErrInvalidCommand, err)
		}
		if errors.Is(err, terminationSvc.ErrRejected) {
			return fmt.Errorf("%w: %v", ErrRejected, err)
		}
		return err
	}
	return nil
}

func classifyRouteError(target string, err error) error {
	if errors.Is(err, repository.ErrExecutionPoolNotFound) {
		return fmt.Errorf("%w: 执行资源池 %q 不存在", ErrRejected, target)
	}
	return fmt.Errorf("规划工作流执行路由失败: %w", err)
}

func validateCommand(command RunRunnerCommand) error {
	if strings.TrimSpace(command.RequestID) == "" {
		return fmt.Errorf("幂等请求标识不能为空")
	}
	if command.RunnerID <= 0 {
		return fmt.Errorf("执行单元 ID 非法: %d", command.RunnerID)
	}
	if args := strings.TrimSpace(command.Params["args"]); args != "" && !json.Valid([]byte(args)) {
		return fmt.Errorf("工作流执行参数必须是合法 JSON")
	}
	return nil
}

func (s *service) resolveProgram(ctx context.Context, codebookID int64,
	kind domain.ProgramKind) (programSvc.Resolution, error) {
	spec := &domain.ProgramSpec{Kind: kind}
	switch kind {
	case domain.ProgramInline:
		spec.Inline = &domain.InlineProgramSpec{CodebookID: codebookID}
	case domain.ProgramProject:
		spec.Project = &domain.ProjectProgramSpec{EntryCodebookID: codebookID}
	}
	return s.programs.Resolve(ctx, spec)
}

func (s *service) buildParams(runner domain.Runner, command RunRunnerCommand) (map[string]string, error) {
	variables, err := mergeVariables(runner.Variables, command.Variables)
	if err != nil {
		return nil, err
	}
	variablesJSON, err := json.Marshal(variables)
	if err != nil {
		return nil, fmt.Errorf("序列化执行单元变量失败: %w", err)
	}
	params := make(map[string]string, len(command.Params)+1)
	for key, value := range command.Params {
		params[key] = value
	}
	if strings.TrimSpace(params["args"]) == "" {
		params["args"] = "{}"
	}
	params["variables"] = string(variablesJSON)
	return params, nil
}

type runtimeVariable struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Secret bool   `json:"secret"`
}

func mergeVariables(defaults []domain.RunnerVariable, overrides map[string]string) ([]runtimeVariable, error) {
	values := make(map[string]domain.RunnerVariable, len(defaults)+len(overrides))
	keys := make([]string, 0, len(defaults)+len(overrides))
	for _, variable := range defaults {
		values[variable.Key] = variable
		keys = append(keys, variable.Key)
	}
	for key, value := range overrides {
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("临时变量名称不能为空")
		}
		variable, exists := values[key]
		if !exists {
			keys = append(keys, key)
		}
		variable.Key = key
		variable.Value = value
		values[key] = variable
	}
	result := make([]runtimeVariable, 0, len(keys))
	for _, key := range keys {
		variable := values[key]
		result = append(result, runtimeVariable{
			Key: variable.Key, Value: variable.Value, Secret: variable.Secret,
		})
	}
	return result, nil
}

func (s *service) invoke(ctx context.Context, execution domain.TaskExecution) {
	state, err := s.invoker.Run(ctx, execution)
	if err != nil {
		state = domain.ExecutionState{
			ID: execution.ID, TaskID: execution.Task.ID, TaskName: execution.Task.Name,
			Status:     domain.TaskExecutionStatusFailed,
			TaskResult: fmt.Sprintf("调用执行节点失败: %v", err),
		}
	}
	if updateErr := s.executions.UpdateState(ctx, state); updateErr != nil {
		s.logger.Error("更新工作流执行状态失败",
			elog.Int64("executionID", execution.ID), elog.FieldErr(updateErr))
	}
}
