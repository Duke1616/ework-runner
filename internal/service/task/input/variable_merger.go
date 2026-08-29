package input

import (
	"fmt"
	"strings"

	"github.com/Duke1616/etask/internal/pkg/variable"
)

// VariableSource 描述变量来源，用于日志、测试和排查优先级问题。
type VariableSource string

const (
	// VariableSourceRunner 表示 Runner 提供的有效变量。
	VariableSourceRunner VariableSource = "runner"
	// VariableSourceTask 表示任务配置中的结构化变量。
	VariableSourceTask VariableSource = "task"
	// VariableSourceBinding 表示绑定解析得到的结构化变量。
	VariableSourceBinding VariableSource = "binding"
)

// VariableLayer 表示一组具有相同优先级来源的变量。
type VariableLayer struct {
	Source VariableSource
	Items  []variable.Item
}

// VariableMerger 合并执行阶段使用的结构化变量。
type VariableMerger struct{}

// Merge 保留首次出现的变量顺序，后续来源只替换变量内容。
func (VariableMerger) Merge(layers ...VariableLayer) ([]variable.Item, error) {
	values := make(map[string]variable.Item)
	order := make([]string, 0)
	for _, layer := range layers {
		for _, item := range layer.Items {
			item.Key = strings.TrimSpace(item.Key)
			if item.Key == "" {
				return nil, fmt.Errorf("%s变量名称不能为空", layer.Source)
			}
			if _, exists := values[item.Key]; !exists {
				order = append(order, item.Key)
			}
			values[item.Key] = item
		}
	}
	result := make([]variable.Item, 0, len(order))
	for _, key := range order {
		result = append(result, values[key])
	}
	return result, nil
}
