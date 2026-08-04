package binding

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/Duke1616/etask/internal/domain"
	codebookSvc "github.com/Duke1616/etask/internal/service/codebook"
	runnerSvc "github.com/Duke1616/etask/internal/service/runner"
	"github.com/samber/lo"
)

const (
	CodebookBinding = "codebook"
	RunnerBinding   = "runner"
)

func NewScriptBindingResolvers(codebookSvc codebookSvc.Service, runnerSvc runnerSvc.Service) *Registry {
	return NewRegistry().
		Register(CodebookBinding, CodebookResolver{svc: codebookSvc}).
		Register(RunnerBinding, RunnerResolver{svc: runnerSvc})
}

type CodebookResolver struct {
	svc codebookSvc.Service
}

func (r CodebookResolver) Resolve(ctx context.Context, req ResolveRequest) (string, error) {
	id, err := parseID(req.Value, req.ParamKey)
	if err != nil {
		return "", err
	}

	codebook, err := r.svc.GetByID(ctx, id)
	if err != nil {
		return "", fmt.Errorf("获取代码资源失败: %w", err)
	}
	return codebook.Code, nil
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

	variables := lo.Map(vars, func(v domain.RunnerVariable, _ int) variable {
		return variable{
			Key:    v.Key,
			Value:  v.Value,
			Secret: v.Secret,
		}
	})

	bytes, err := json.Marshal(variables)
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

type variable struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Secret bool   `json:"secret"`
}

var _ Resolver = CodebookResolver{}
var _ Resolver = RunnerResolver{}
