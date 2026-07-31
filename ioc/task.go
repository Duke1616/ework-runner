package ioc

import (
	"github.com/Duke1616/etask/internal/compensator"
	poolSvc "github.com/Duke1616/etask/internal/service/pool"
)

func InitTasks(
	t1 *compensator.RetryCompensator,
	t2 *compensator.RescheduleCompensator,
	t3 *compensator.InterruptCompensator,
	t4 *CompleteConsumer,
	t5 *poolSvc.Syncer,
	t6 *AgentEventConsumer,
) []Task {
	return []Task{
		t1,
		t2,
		t3,
		t4,
		t5,
		t6,
	}
}
