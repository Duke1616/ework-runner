package invoker

import (
	"context"
	"fmt"

	"github.com/Duke1616/etask/internal/domain"
)

var _ Invoker = &LocalInvoker{}

type (
	LocalExecuteFunc func(ctx context.Context, execution domain.TaskExecution) (domain.ExecutionState, error)
)

// LocalInvoker 本地调用器
// 主要用来本地运行，辅助测试等
// 在必要时刻，用户可以自己部署 scheduler，而后使用该调用器
type LocalInvoker struct {
	executeFuncs map[string]LocalExecuteFunc
}

func NewLocalInvoker(executeFuncs map[string]LocalExecuteFunc) *LocalInvoker {
	return &LocalInvoker{executeFuncs: executeFuncs}
}

func (l *LocalInvoker) Name() string {
	return "LOCAL"
}

func (l *LocalInvoker) Run(ctx context.Context, execution domain.TaskExecution) (domain.ExecutionState, error) {
	fn, ok := l.executeFuncs[execution.Task.Name]
	if !ok {
		return domain.ExecutionState{}, fmt.Errorf("未注册方法：%s", execution.Task.Name)
	}
	// 直接执行本地函数
	return fn(ctx, execution)
}

func (l *LocalInvoker) Terminate(context.Context, domain.TaskExecution, string) error { return nil }
