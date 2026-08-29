package manager

import (
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestToExecutionParametersVO(t *testing.T) {
	execution := domain.TaskExecution{
		ID: 42,
		Task: domain.Task{
			GrpcConfig: &domain.GrpcConfig{Params: map[string]string{
				"zeta":   "task",
				"region": "task-region",
			}},
			ScheduleParams: map[string]string{
				"region": "schedule-region",
				"page":   "2",
			},
			MaxExecutionSeconds: 300,
		},
		ParamOverrides: map[string]string{
			"zeta": "manual",
		},
	}

	got := toExecutionParametersVO(execution)
	require.Equal(t, int64(42), got.ExecutionID)
	require.Equal(t, 1, got.ManualOverrideCount)
	require.Equal(t, 2, got.ScheduleOverrideCount)
	require.Equal(t, []string{"page", "region", "zeta"},
		[]string{got.Parameters[0].Key, got.Parameters[1].Key, got.Parameters[2].Key})

	byKey := make(map[string]ExecutionParameterVO, len(got.Parameters))
	for _, parameter := range got.Parameters {
		byKey[parameter.Key] = parameter
	}
	require.Equal(t, "schedule-region", byKey["region"].Value)
	require.Equal(t, "SCHEDULE_OVERRIDE", byKey["region"].Source)
	require.True(t, byKey["region"].ScheduleOverride)
	require.Equal(t, "manual", byKey["zeta"].Value)
	require.Equal(t, "MANUAL_OVERRIDE", byKey["zeta"].Source)
	require.True(t, byKey["zeta"].ManualOverride)
}

func TestToExecutionParametersVOHandlesEmptySnapshot(t *testing.T) {
	got := toExecutionParametersVO(domain.TaskExecution{ID: 7})
	require.Equal(t, int64(7), got.ExecutionID)
	require.Empty(t, got.Parameters)
	require.Zero(t, got.ManualOverrideCount)
	require.Zero(t, got.ScheduleOverrideCount)
}

func TestToExecutionParametersVOMasksSecretVariables(t *testing.T) {
	got := toExecutionParametersVO(domain.TaskExecution{
		ID: 8,
		Variables: &domain.ExecutionVariableSet{Items: []domain.RunnerVariable{
			{Key: "public", Value: "visible"},
			{Key: "token", Value: "top-secret", Secret: true},
		}},
	})
	byKey := make(map[string]ExecutionParameterVO, len(got.Parameters))
	for _, parameter := range got.Parameters {
		byKey[parameter.Key] = parameter
	}
	require.Equal(t, `[{"key":"public","value":"visible","secret":false},{"key":"token","value":"[已脱敏]","secret":true}]`, byKey["variables"].Value)
	require.NotContains(t, byKey["variables"].Value, "top-secret")
}
