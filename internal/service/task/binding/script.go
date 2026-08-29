package binding

import (
	"context"
	"fmt"
	"strconv"

	"github.com/Duke1616/etask/internal/pkg/variable"
	runnerSvc "github.com/Duke1616/etask/internal/service/runner"
)

const (
	RunnerBinding = "runner"
)

func NewScriptBindingResolvers(runnerSvc runnerSvc.Service) *Registry {
	return NewRegistry().Register(RunnerBinding, RunnerResolver{svc: runnerSvc})
}

type RunnerResolver struct {
	svc runnerSvc.Service
}

// Resolve 将执行器变量作为结构化结果返回，由执行服务统一合并并持久化。
func (r RunnerResolver) Resolve(ctx context.Context, req ResolveRequest) (ResolveResult, error) {
	id, err := parseID(req.Value, req.ParamKey)
	if err != nil {
		return ResolveResult{}, err
	}

	vars, err := r.svc.ListMergedVariables(ctx, id)
	if err != nil {
		return ResolveResult{}, fmt.Errorf("获取执行器变量失败: %w", err)
	}
	items := make([]variable.Item, len(vars))
	copy(items, vars)
	return ResolveResult{Variables: items}, nil
}

func parseID(rawID string, param string) (int64, error) {
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("参数 %s 的绑定 ID 非法: %q", param, rawID)
	}
	return id, nil
}

var _ Resolver = RunnerResolver{}
