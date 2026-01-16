package event

import "github.com/Duke1616/ework-runner/internal/domain"

type Event struct {
	TaskID         int64                      `json:"taskId"`
	ExecID         int64                      `json:"execId"`
	Version        int64                      `json:"version"`
	ScheduleNodeID string                     `json:"scheduleNodeId"`
	ExecStatus     domain.TaskExecutionStatus `json:"execStatus"`
	Name           string                     `json:"name"`
}
