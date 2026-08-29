package pool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	"github.com/Duke1616/etask/internal/repository"
	"github.com/samber/lo"
)

// HandlerMetadata 描述资源池声明的一个执行 Handler。
type HandlerMetadata struct {
	Name       string              `json:"name"`
	Parameters []ParameterMetadata `json:"metadata"`
}

// ParameterMetadata 描述 Handler 的一个参数及其运行时语义。
type ParameterMetadata struct {
	Key                string `json:"key"`
	Role               string `json:"role"`
	RuntimeOverridable bool   `json:"runtime_overridable"`
}

// HandlerMetadataProvider 只提供 Handler 元数据查询，不负责资源池授权或绑定管理。
type HandlerMetadataProvider interface {
	// FindHandlerMetadata 查询资源池中指定 Handler 的元数据。
	FindHandlerMetadata(ctx context.Context, req CheckBindingRequest) (HandlerMetadata, error)
	// RuntimeOverridableParameterKeys 返回允许启动时覆盖的参数键。
	RuntimeOverridableParameterKeys(ctx context.Context, req CheckBindingRequest) (map[string]struct{}, error)
	// VariableParameterKeys 返回承担结构化变量语义的参数键。
	VariableParameterKeys(ctx context.Context, req CheckBindingRequest) (map[string]struct{}, error)
}

// handlerMetadataProvider 负责读取和筛选资源池 Handler 元数据。
type handlerMetadataProvider struct {
	poolRepo repository.ExecutionPoolRepository
}

// NewHandlerMetadataProvider 创建 Handler 元数据查询器。
func NewHandlerMetadataProvider(poolRepo repository.ExecutionPoolRepository) HandlerMetadataProvider {
	return &handlerMetadataProvider{poolRepo: poolRepo}
}

// FindHandlerMetadata 查询指定资源池中的 Handler 元数据。
func (p *handlerMetadataProvider) FindHandlerMetadata(ctx context.Context, req CheckBindingRequest) (HandlerMetadata, error) {
	poolName, handlerName, err := normalizeBindingKey(req.PoolName, req.HandlerName)
	if err != nil {
		return HandlerMetadata{}, err
	}
	if handlerName == "" {
		return HandlerMetadata{}, fmt.Errorf("%w: handler 不能为空", errs.ErrInvalidParameter)
	}
	executionPool, err := p.poolRepo.Find(ctx, poolName)
	if err != nil {
		return HandlerMetadata{}, err
	}
	handlers, _, err := parseHandlerMetadata(executionPool)
	if err != nil {
		return HandlerMetadata{}, err
	}
	handler, err := findHandlerMetadata(handlers, handlerName)
	if err != nil {
		return HandlerMetadata{}, fmt.Errorf("%w: handler %s 不属于资源池 %s", errs.ErrInvalidParameter, handlerName, poolName)
	}
	return handler, nil
}

// RuntimeOverridableParameterKeys 返回允许启动时覆盖的参数键。
func (p *handlerMetadataProvider) RuntimeOverridableParameterKeys(ctx context.Context,
	req CheckBindingRequest) (map[string]struct{}, error) {
	handler, err := p.FindHandlerMetadata(ctx, req)
	if err != nil {
		return nil, err
	}
	return collectParameterKeys(handler, func(parameter ParameterMetadata) bool {
		return parameter.RuntimeOverridable
	}), nil
}

// VariableParameterKeys 返回承担结构化变量语义的参数键。
func (p *handlerMetadataProvider) VariableParameterKeys(ctx context.Context,
	req CheckBindingRequest) (map[string]struct{}, error) {
	handler, err := p.FindHandlerMetadata(ctx, req)
	if err != nil {
		return nil, err
	}
	return collectParameterKeys(handler, func(parameter ParameterMetadata) bool {
		return strings.TrimSpace(parameter.Role) == "variables"
	}), nil
}

// collectParameterKeys 从 Handler 参数元数据中提取满足条件的参数键。
func collectParameterKeys(handler HandlerMetadata, match func(ParameterMetadata) bool) map[string]struct{} {
	keys := make(map[string]struct{})
	for _, parameter := range handler.Parameters {
		key := strings.TrimSpace(parameter.Key)
		if key != "" && match(parameter) {
			keys[key] = struct{}{}
		}
	}
	return keys
}

func parseHandlerMetadata(pool domain.ExecutionPool) ([]HandlerMetadata, bool, error) {
	if pool.Metadata == nil {
		return nil, false, nil
	}
	raw := strings.TrimSpace(pool.Metadata["supported_handlers"])
	if raw == "" {
		return nil, false, nil
	}
	var handlers []HandlerMetadata
	if err := json.Unmarshal([]byte(raw), &handlers); err != nil {
		return nil, false, fmt.Errorf("解析资源池 %s Handler 元数据失败: %w", pool.Name, err)
	}
	return handlers, len(handlers) > 0, nil
}

func findHandlerMetadata(handlers []HandlerMetadata, handlerName string) (HandlerMetadata, error) {
	handler, found := lo.Find(handlers, func(handler HandlerMetadata) bool {
		return domain.NormalizeExecutionPoolHandlerName(handler.Name) == handlerName
	})
	if found {
		return handler, nil
	}
	return HandlerMetadata{}, fmt.Errorf("%w: handler %s 不属于资源池", errs.ErrInvalidParameter, handlerName)
}

var _ HandlerMetadataProvider = (*handlerMetadataProvider)(nil)
