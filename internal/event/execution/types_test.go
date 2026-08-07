package execution

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
)

func TestNewCommand(t *testing.T) {
	testCases := []struct {
		name    string
		exec    domain.TaskExecution
		wantErr string
	}{
		{name: "正式任务", exec: commandExecution(domain.TaskExecutionSourceTask, 30)},
		{name: "试运行允许任务 ID 为空", exec: commandExecution(domain.TaskExecutionSourceCodebookPreview, 0)},
		{name: "工作流允许任务 ID 为空", exec: commandExecution(domain.TaskExecutionSourceWorkflow, 0)},
		{name: "拒绝未声明来源", exec: commandExecution("", 30), wantErr: "来源非法"},
		{name: "拒绝缺少处理器配置", exec: domain.TaskExecution{ID: 10, TenantID: 20}, wantErr: "缺少处理器配置"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			command, err := NewCommand(testCase.exec, "dispatch-1")
			if testCase.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
					t.Fatalf("NewCommand() 错误 = %v, 期望包含 %q", err, testCase.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewCommand() 返回意外错误: %v", err)
			}
			if command.Source != testCase.exec.Source || command.TaskID != testCase.exec.Task.ID ||
				command.Handler != "shell" {
				t.Fatalf("NewCommand() = %#v", command)
			}
		})
	}
}

func TestCommandProgramJSONRoundTrip(t *testing.T) {
	testCases := []struct {
		name    string
		program *domain.Program
	}{
		{
			name:    "INLINE",
			program: domain.NewInlineProgram("echo ok"),
		},
		{
			name: "PROJECT",
			program: &domain.Program{Kind: domain.ProgramProject, Project: &domain.ProjectProgram{
				Source: domain.ProjectSourceRef{
					SourceID: 1, ProjectID: 2, SourceRevision: 3,
					Digest: strings.Repeat("a", 64), BlobChecksum: strings.Repeat("b", 64),
					Size: 128, Format: "tar.zst", FormatVersion: 1,
				},
				EntryPoint: "roles/deploy/tasks/main.yml",
			}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			execution := commandExecution(domain.TaskExecutionSourceTask, 30)
			execution.Program = testCase.program
			command, err := NewCommand(execution, "dispatch-1")
			if err != nil {
				t.Fatalf("NewCommand() 返回意外错误: %v", err)
			}

			data, err := json.Marshal(command)
			if err != nil {
				t.Fatalf("序列化 Command 失败: %v", err)
			}
			var decoded Command
			if err = json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("反序列化 Command 失败: %v", err)
			}
			if err = decoded.Validate(); err != nil {
				t.Fatalf("反序列化后的 Command 校验失败: %v", err)
			}
			if !reflect.DeepEqual(decoded.Execution().Program, testCase.program) {
				t.Fatalf("Kafka 往返后的 Program = %#v, 期望 %#v", decoded.Execution().Program, testCase.program)
			}
		})
	}
}

func commandExecution(source domain.TaskExecutionSource, taskID int64) domain.TaskExecution {
	return domain.TaskExecution{
		ID: 10, TenantID: 20, Source: source,
		Task: domain.Task{ID: taskID, Name: "测试任务", GrpcConfig: &domain.GrpcConfig{
			HandlerName: "shell", Params: map[string]string{"code": "echo ok"},
		}},
	}
}
