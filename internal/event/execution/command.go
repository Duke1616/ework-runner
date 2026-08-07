// Package execution 定义 Scheduler 与 Kafka Agent 之间的执行协议。
package execution

import (
	"fmt"
	"strings"

	"github.com/Duke1616/etask/internal/domain"
)

// Command 是调度中心发送给独立 Agent 的不可变执行命令。
type Command struct {
	DispatchID  string                     `json:"dispatch_id"`
	ExecutionID int64                      `json:"execution_id"`
	TaskID      int64                      `json:"task_id"`
	TenantID    int64                      `json:"tenant_id"`
	Source      domain.TaskExecutionSource `json:"source"`
	TaskName    string                     `json:"task_name"`
	Handler     string                     `json:"handler"`
	Params      map[string]string          `json:"params"`
	Artifacts   []domain.ArtifactRef       `json:"artifacts"`
	Program     *domain.Program            `json:"program,omitempty"`
}

// NewCommand 将领域执行快照转换为 Kafka 协议，并在发布前校验协议约束。
func NewCommand(execution domain.TaskExecution, dispatchID string) (Command, error) {
	if execution.Task.GrpcConfig == nil {
		return Command{}, fmt.Errorf("Agent 执行快照缺少处理器配置")
	}
	command := Command{
		DispatchID:  dispatchID,
		ExecutionID: execution.ID,
		TaskID:      execution.Task.ID,
		TenantID:    execution.TenantID,
		Source:      execution.Source,
		TaskName:    execution.Task.Name,
		Handler:     execution.Task.GrpcConfig.HandlerName,
		Params:      execution.GRPCParams(),
		Artifacts:   execution.Artifacts,
		Program:     execution.Program,
	}
	if err := command.Validate(); err != nil {
		return Command{}, err
	}
	return command, nil
}

// Validate 校验 Agent 执行命令的协议约束。
func (c Command) Validate() error {
	if strings.TrimSpace(c.DispatchID) == "" || c.ExecutionID <= 0 || c.TenantID <= 0 ||
		strings.TrimSpace(c.Handler) == "" {
		return fmt.Errorf("Agent 执行命令身份信息非法: dispatch_id=%q execution_id=%d tenant_id=%d handler=%q",
			c.DispatchID, c.ExecutionID, c.TenantID, c.Handler)
	}
	if !c.Source.IsValid() {
		return fmt.Errorf("Agent 执行命令来源非法: execution_id=%d source=%s", c.ExecutionID, c.Source)
	}
	if c.Source == domain.TaskExecutionSourceTask && c.TaskID <= 0 {
		return fmt.Errorf("Agent 正式任务命令缺少 task_id: execution_id=%d source=%s", c.ExecutionID, c.Source)
	}
	if c.Program != nil {
		if err := c.Program.Validate(); err != nil {
			return fmt.Errorf("Agent 执行命令程序非法: execution_id=%d: %w", c.ExecutionID, err)
		}
	}
	return nil
}

// Execution 将消息协议转换为 Agent 执行引擎使用的领域快照。
func (c Command) Execution() domain.TaskExecution {
	return domain.TaskExecution{
		ID: c.ExecutionID, TenantID: c.TenantID, Source: c.Source, Artifacts: c.Artifacts, Program: c.Program,
		Task: domain.Task{
			ID: c.TaskID, TenantID: c.TenantID, Name: c.TaskName,
			GrpcConfig: &domain.GrpcConfig{HandlerName: c.Handler, Params: c.Params},
		},
	}
}

// DispatchNodeID 构造写入执行快照的 Kafka 派发标记。
func DispatchNodeID(topic, dispatchID string) string {
	return "agent:" + topic + ":" + dispatchID
}
