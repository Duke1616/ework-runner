package domain

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTaskExecutionGRPCParamsOnlyContainsHandlerAndScheduleParams(t *testing.T) {
	execution := TaskExecution{Task: Task{
		GrpcConfig: &GrpcConfig{Params: map[string]string{
			"region": "task-region",
			"args":   "{}",
		}},
		ScheduleParams: map[string]string{
			"region": "schedule-region",
			"page":   "2",
		},
		MaxExecutionSeconds: 300,
	}}

	require.Equal(t, map[string]string{
		"region": "schedule-region",
		"args":   "{}",
		"page":   "2",
	}, execution.GRPCParams())
}
