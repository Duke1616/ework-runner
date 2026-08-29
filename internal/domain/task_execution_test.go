package domain

import (
	"testing"

	executorv1 "github.com/Duke1616/etask/api/proto/gen/etask/executor/v1"
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

func TestTaskExecutionStatus_ProtoConversion(t *testing.T) {
	testCases := []struct {
		name        string
		domain      TaskExecutionStatus
		proto       executorv1.ExecutionStatus
		wantRoundOk bool
	}{
		{name: "RUNNING 正常互转", domain: TaskExecutionStatusRunning, proto: executorv1.ExecutionStatus_RUNNING, wantRoundOk: true},
		{name: "SUCCESS 正常互转", domain: TaskExecutionStatusSuccess, proto: executorv1.ExecutionStatus_SUCCESS, wantRoundOk: true},
		{name: "FAILED 正常互转", domain: TaskExecutionStatusFailed, proto: executorv1.ExecutionStatus_FAILED, wantRoundOk: true},
		{name: "FAILED_RETRYABLE 正常互转", domain: TaskExecutionStatusFailedRetryable, proto: executorv1.ExecutionStatus_FAILED_RETRYABLE, wantRoundOk: true},
		{name: "FAILED_RESCHEDULED 准确映射到 FAILED_RESCHEDULABLE", domain: TaskExecutionStatusFailedRescheduled, proto: executorv1.ExecutionStatus_FAILED_RESCHEDULABLE, wantRoundOk: true},
		{name: "CANCELLED 正常互转", domain: TaskExecutionStatusCancelled, proto: executorv1.ExecutionStatus_CANCELLED, wantRoundOk: true},
		{name: "WAITING_PULL 兜底为 UNKNOWN", domain: TaskExecutionStatusWaitingPull, proto: executorv1.ExecutionStatus_UNKNOWN, wantRoundOk: false},
		{name: "PREPARE 兜底为 UNKNOWN", domain: TaskExecutionStatusPrepare, proto: executorv1.ExecutionStatus_UNKNOWN, wantRoundOk: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotProto := tc.domain.ToProto()
			require.Equal(t, tc.proto, gotProto)
			if tc.wantRoundOk {
				gotDomain := TaskExecutionStatusFromProto(gotProto)
				require.Equal(t, tc.domain, gotDomain)
			}
		})
	}
}
