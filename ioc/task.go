package ioc

import (
	"github.com/Duke1616/ecmdb/internal/compensator"
)

func InitTasks(
	t1 *compensator.RetryCompensator,
	t2 *compensator.RescheduleCompensator,
	t4 *compensator.InterruptCompensator,
) []Task {
	return []Task{
		t1,
		t2,
		t4,
	}
}
