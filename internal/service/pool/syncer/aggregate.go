package syncer

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/samber/lo"
)

// aggregatePoolInstances 将同一资源池的节点注册信息收敛为一个稳定快照。
func aggregatePoolInstances(instances []domain.ExecutionPool) (domain.ExecutionPool, error) {
	if len(instances) == 0 {
		return domain.ExecutionPool{}, fmt.Errorf("资源池没有在线实例")
	}

	result := instances[0]
	result.Metadata = sharedMetadata(result.Metadata)
	merged := newHandlerSet()
	if err := merged.Add(result.Metadata["supported_handlers"]); err != nil {
		return domain.ExecutionPool{}, err
	}

	for _, instance := range instances[1:] {
		if err := validatePoolConfig(result, instance); err != nil {
			return domain.ExecutionPool{}, err
		}
		if err := merged.Add(instance.Metadata["supported_handlers"]); err != nil {
			return domain.ExecutionPool{}, err
		}
	}

	result.Metadata["supported_handlers"] = merged.JSON()
	return result, nil
}

func validatePoolConfig(base, candidate domain.ExecutionPool) error {
	if base.Kind != candidate.Kind || base.Transport != candidate.Transport ||
		base.DispatchMode != candidate.DispatchMode || base.IsolationLevel != candidate.IsolationLevel {
		return fmt.Errorf("资源池配置不一致: %s/%s/%s/%s 与 %s/%s/%s/%s",
			base.Kind, base.Transport, base.DispatchMode, base.IsolationLevel,
			candidate.Kind, candidate.Transport, candidate.DispatchMode, candidate.IsolationLevel)
	}
	if base.Kind == domain.ExecutionPoolKindAgent &&
		strings.TrimSpace(base.Metadata["topic"]) != strings.TrimSpace(candidate.Metadata["topic"]) {
		return fmt.Errorf("Agent Topic 不一致: %s 与 %s", base.Metadata["topic"], candidate.Metadata["topic"])
	}
	return nil
}

func sharedMetadata(metadata map[string]string) map[string]string {
	return lo.OmitByKeys(metadata, []string{"instance_id", "address"})
}

type handlerSet struct {
	items map[string]json.RawMessage
}

func newHandlerSet() *handlerSet {
	return &handlerSet{items: make(map[string]json.RawMessage)}
}

func (s *handlerSet) Add(raw string) error {
	var handlers []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &handlers); err != nil {
		return fmt.Errorf("解析资源池 Handler 元数据失败: %w", err)
	}
	for _, handler := range handlers {
		var identity struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(handler, &identity); err != nil || strings.TrimSpace(identity.Name) == "" {
			return fmt.Errorf("资源池 Handler 元数据缺少合法名称")
		}
		canonical, err := canonicalJSON(handler)
		if err != nil {
			return fmt.Errorf("规范化 Handler %s 元数据失败: %w", identity.Name, err)
		}
		if old, exists := s.items[identity.Name]; exists && string(old) != string(canonical) {
			return fmt.Errorf("Handler %s 在不同实例中的参数定义不一致", identity.Name)
		}
		s.items[identity.Name] = canonical
	}
	return nil
}

func (s *handlerSet) JSON() string {
	names := slices.Sorted(maps.Keys(s.items))
	items := lo.Map(names, func(name string, _ int) json.RawMessage {
		return s.items[name]
	})
	data, _ := json.Marshal(items)
	return string(data)
}

func canonicalJSON(raw json.RawMessage) (json.RawMessage, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return json.Marshal(value)
}
