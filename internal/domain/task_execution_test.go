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

func TestTask_WithScheduleParams(t *testing.T) {
	testCases := []struct {
		name         string
		initial      map[string]string
		override     map[string]string
		want         map[string]string
		mutateOrigin bool
	}{
		{
			name:     "智能合并覆盖与新增字段",
			initial:  map[string]string{"k1": "v1", "k2": "v2"},
			override: map[string]string{"k2": "v2_new", "k3": "v3"},
			want:     map[string]string{"k1": "v1", "k2": "v2_new", "k3": "v3"},
		},
		{
			name:     "传入nil保留原参数",
			initial:  map[string]string{"k1": "v1"},
			override: nil,
			want:     map[string]string{"k1": "v1"},
		},
		{
			name:     "传入空map清空参数",
			initial:  map[string]string{"k1": "v1"},
			override: map[string]string{},
			want:     map[string]string{},
		},
		{
			name:         "深拷贝隔离_修改原任务不影响派生快照",
			initial:      map[string]string{"k1": "v1"},
			override:     map[string]string{"k2": "v2"},
			mutateOrigin: true,
			want:         map[string]string{"k1": "v1", "k2": "v2"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			task := Task{
				ID:             1,
				ScheduleParams: tc.initial,
			}

			snapshot := task.WithScheduleParams(tc.override)

			if tc.mutateOrigin {
				task.ScheduleParams["k1"] = "mutated"
				task.ScheduleParams["k999"] = "leak"
			}

			require.Equal(t, tc.want, snapshot.ScheduleParams)
		})
	}
}
