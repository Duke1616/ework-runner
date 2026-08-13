package ioc

import (
	"github.com/Duke1616/etask/internal/compensator"
	poolSvc "github.com/Duke1616/etask/internal/service/pool"
	internalSSE "github.com/Duke1616/etask/internal/sse"
)

func InitTasks(
	t1 *compensator.RetryCompensator,
	t2 *compensator.RescheduleCompensator,
	t3 *compensator.InterruptCompensator,
	t4 *compensator.TerminationCompensator,
	t5 *CompleteConsumer,
	t6 *poolSvc.Syncer,
	t7 *AgentEventConsumer,
	t8 *internalSSE.Hubs,
) []Task {
	return []Task{
		t1,
		t2,
		t3,
		t4,
		t5,
		t6,
		t7,
		t8,
	}
}
