package runner

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeParameterDefaultsKeepsJSONValuesStructuredUntilDispatch(t *testing.T) {
	defaults := map[string]json.RawMessage{
		"forks":   json.RawMessage(`20`),
		"check":   json.RawMessage(`true`),
		"vars":    json.RawMessage(`[{"key":"env","value":"prod"}]`),
		"message": json.RawMessage(`"hello"`),
	}

	got, err := MergeParameterDefaults(defaults, map[string]string{"forks": "8", "check": ""})
	require.NoError(t, err)
	require.Equal(t, "8", got["forks"])
	require.Equal(t, "", got["check"])
	require.Equal(t, `[{"key":"env","value":"prod"}]`, got["vars"])
	require.Equal(t, "hello", got["message"])
}

func TestParameterDefaultValueRejectsInvalidJSON(t *testing.T) {
	_, err := ParameterDefaultValue(json.RawMessage(`{"forks":`))
	require.Error(t, err)
}
