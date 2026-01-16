package ioc

import (
	"github.com/Duke1616/ework-runner/internal/compensator"
)

func InitTasks(
	t1 *compensator.RetryCompensator,
	t2 *compensator.RescheduleCompensator,
	t3 *compensator.InterruptCompensator,
	t4 *CompleteConsumer,
) []Task {
	return []Task{
		t1,
		t2,
		t3,
		t4,
	}
}
