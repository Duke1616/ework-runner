package event

import (
	"strings"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/pkg/grpc/registry"
)

func TestCommandValidate(t *testing.T) {
	base := ExecuteCommand{DispatchID: "dispatch-1", ExecutionID: 10, TenantID: 20, Handler: "shell"}
	testCases := []struct {
		name string
		cmd  ExecuteCommand
		want string
	}{
		{name: "正式任务必须有 TaskID", cmd: ExecuteCommand{DispatchID: "d", ExecutionID: 10, TenantID: 20, Handler: "shell", Source: domain.TaskExecutionSourceTask}, want: "缺少 task_id"},
		{name: "试运行允许 TaskID 为零", cmd: func() ExecuteCommand { c := base; c.Source = domain.TaskExecutionSourceCodebookPreview; return c }()},
		{name: "工作流允许 TaskID 为零", cmd: func() ExecuteCommand { c := base; c.Source = domain.TaskExecutionSourceWorkflow; return c }()},
		{name: "执行来源必须明确", cmd: base, want: "来源非法"},
		{name: "拒绝未知执行来源", cmd: func() ExecuteCommand { c := base; c.Source = "UNKNOWN"; return c }(), want: "来源非法"},
		{name: "缺少派发 ID", cmd: ExecuteCommand{ExecutionID: 10, TenantID: 20, Handler: "shell", Source: domain.TaskExecutionSourceCodebookPreview}, want: "身份信息非法"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.cmd.Validate()
			if testCase.want == "" {
				if err != nil {
					t.Fatalf("validateCommand() 返回意外错误: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("validateCommand() 错误 = %v, 期望包含 %q", err, testCase.want)
			}
		})
	}
}

func TestControlConsumerGroupUsesUniqueAddressWhenIDMissing(t *testing.T) {
	group := controlConsumerGroup("agent-shell", registry.ServiceInstance{Address: "instance-1"})
	if group != "agent-control-agent-shell-instance-1" {
		t.Fatalf("controlConsumerGroup() = %q", group)
	}
}

func TestControlCommandValidateDoesNotDuplicateTenantIdentity(t *testing.T) {
	command := ControlCommand{ExecutionID: 10, Reason: "管理员强制结束"}
	if err := command.Validate(); err != nil {
		t.Fatalf("ControlCommand.Validate() 返回意外错误: %v", err)
	}
}
