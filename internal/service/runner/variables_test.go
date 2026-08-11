package runner

import (
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestMergeVariablesPreservesOrderAndOverrideMetadata(t *testing.T) {
	result, err := MergeVariables(
		[]domain.RunnerVariable{
			{Key: "REGION", Value: "cn"},
			{Key: "TOKEN", Value: "default", Secret: true},
		},
		[]domain.RunnerVariable{
			{Key: "TOKEN", Value: "preview", Secret: false},
			{Key: "DEBUG", Value: "true"},
		},
	)

	require.NoError(t, err)
	require.Equal(t, []domain.RunnerVariable{
		{Key: "REGION", Value: "cn"},
		{Key: "TOKEN", Value: "preview", Secret: false},
		{Key: "DEBUG", Value: "true"},
	}, result)
}

func TestMergeVariableValuesIsDeterministicAndPreservesSecret(t *testing.T) {
	result, err := MergeVariableValues(
		[]domain.RunnerVariable{{Key: "TOKEN", Value: "default", Secret: true}},
		map[string]string{"ZONE": "b", "DEBUG": "true", "TOKEN": "workflow"},
	)

	require.NoError(t, err)
	require.Equal(t, []domain.RunnerVariable{
		{Key: "TOKEN", Value: "workflow", Secret: true},
		{Key: "DEBUG", Value: "true"},
		{Key: "ZONE", Value: "b"},
	}, result)
}
