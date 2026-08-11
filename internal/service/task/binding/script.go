package binding

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

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

func (r RunnerResolver) Resolve(ctx context.Context, req ResolveRequest) (string, error) {
	id, err := parseID(req.Value, req.ParamKey)
	if err != nil {
		return "", err
	}

	vars, err := r.svc.ListMergedVariables(ctx, id)
	if err != nil {
		return "", fmt.Errorf("获取执行器变量失败: %w", err)
	}

	bytes, err := json.Marshal(vars)
	if err != nil {
		return "", fmt.Errorf("序列化执行器变量失败: %w", err)
	}
	return string(bytes), nil
}

func parseID(rawID string, param string) (int64, error) {
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("参数 %s 的绑定 ID 非法: %q", param, rawID)
	}
	return id, nil
}

var _ Resolver = RunnerResolver{}
