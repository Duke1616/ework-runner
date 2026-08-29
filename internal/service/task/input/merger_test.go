package input

import (
	"encoding/json"
	"testing"

	"github.com/Duke1616/etask/internal/pkg/variable"
	"github.com/stretchr/testify/require"
)

func TestParameterMergerUsesExplicitPriority(t *testing.T) {
	got, err := (ParameterMerger{}).Merge(ParameterMergeInput{
		RunnerDefaults: map[string]json.RawMessage{
			"region":  json.RawMessage(`"default"`),
			"retries": json.RawMessage(`3`),
		},
		TaskParams:       map[string]string{"region": "task", "name": "demo"},
		BindingParams:    map[string]string{"region": "binding"},
		RuntimeOverrides: map[string]string{"region": "runtime"},
	})

	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"region": "runtime", "retries": "3", "name": "demo",
	}, got)
}

func TestVariableMergerUsesLayerPriorityAndStableOrder(t *testing.T) {
	got, err := (VariableMerger{}).Merge(
		VariableLayer{Source: VariableSourceRunner, Items: []variable.Item{{Key: "REGION", Value: "cn"}, {Key: "TOKEN", Value: "default", Secret: true}}},
		VariableLayer{Source: VariableSourceTask, Items: []variable.Item{{Key: "TOKEN", Value: "task", Secret: false}, {Key: "DEBUG", Value: "true"}}},
		VariableLayer{Source: VariableSourceBinding, Items: []variable.Item{{Key: "TOKEN", Value: "binding", Secret: true}}},
	)

	require.NoError(t, err)
	require.Equal(t, []variable.Item{
		{Key: "REGION", Value: "cn"},
		{Key: "TOKEN", Value: "binding", Secret: true},
		{Key: "DEBUG", Value: "true"},
	}, got)
}

func TestVariableMergerRejectsEmptyKey(t *testing.T) {
	_, err := (VariableMerger{}).Merge(VariableLayer{
		Source: VariableSourceTask,
		Items:  []variable.Item{{Value: "invalid"}},
	})
	require.Error(t, err)
}
