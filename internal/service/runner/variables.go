package runner

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Duke1616/etask/internal/domain"
)

// MergeVariables 保留默认变量顺序；临时变量覆盖同名项，新变量按传入顺序追加。
func MergeVariables(defaults, overrides []domain.RunnerVariable) ([]domain.RunnerVariable, error) {
	return mergeVariables(defaults, overrides, replaceVariable)
}

// MergeVariableValues 合并只携带字符串值的临时变量；同名覆盖保留默认变量的 Secret 属性。
// map 覆盖项按 Key 排序后追加，确保执行快照顺序稳定。
func MergeVariableValues(defaults []domain.RunnerVariable,
	overrides map[string]string) ([]domain.RunnerVariable, error) {
	items := make([]domain.RunnerVariable, 0, len(overrides))
	for _, key := range slices.Sorted(maps.Keys(overrides)) {
		items = append(items, domain.RunnerVariable{Key: key, Value: overrides[key]})
	}
	return mergeVariables(defaults, items, replaceVariableValue)
}

type variableOverride func(current, override domain.RunnerVariable) domain.RunnerVariable

func mergeVariables(defaults, overrides []domain.RunnerVariable,
	applyOverride variableOverride) ([]domain.RunnerVariable, error) {
	result := make([]domain.RunnerVariable, 0, len(defaults)+len(overrides))
	positions := make(map[string]int, len(defaults)+len(overrides))
	for _, variable := range defaults {
		variable, err := normalizeVariable(variable, "默认")
		if err != nil {
			return nil, err
		}
		if position, exists := positions[variable.Key]; exists {
			result[position] = variable
			continue
		}
		positions[variable.Key] = len(result)
		result = append(result, variable)
	}
	for _, variable := range overrides {
		variable, err := normalizeVariable(variable, "临时")
		if err != nil {
			return nil, err
		}
		if position, exists := positions[variable.Key]; exists {
			result[position] = applyOverride(result[position], variable)
			continue
		}
		positions[variable.Key] = len(result)
		result = append(result, variable)
	}
	return result, nil
}

func normalizeVariable(variable domain.RunnerVariable, source string) (domain.RunnerVariable, error) {
	variable.Key = strings.TrimSpace(variable.Key)
	if variable.Key == "" {
		return domain.RunnerVariable{}, fmt.Errorf("%s变量名称不能为空", source)
	}
	return variable, nil
}

func replaceVariable(_ domain.RunnerVariable, override domain.RunnerVariable) domain.RunnerVariable {
	return override
}

func replaceVariableValue(current, override domain.RunnerVariable) domain.RunnerVariable {
	override.Secret = current.Secret
	return override
}
