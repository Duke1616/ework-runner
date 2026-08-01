package compensator

import (
	"context"
	"time"

	"github.com/Duke1616/etask/internal/service/termination"
	"github.com/gotomicro/ego/core/elog"
)

type TerminationConfig struct {
	BatchSize   int
	MinDuration time.Duration
}

type TerminationCompensator struct {
	service termination.Service
	config  TerminationConfig
	logger  *elog.Component
}

func NewTerminationCompensator(service termination.Service,
	config TerminationConfig) *TerminationCompensator {
	if config.BatchSize <= 0 {
		config.BatchSize = 100
	}
	if config.MinDuration <= 0 {
		config.MinDuration = time.Second
	}
	return &TerminationCompensator{
		service: service, config: config,
		logger: elog.DefaultLogger.With(elog.FieldComponentName("compensator.termination")),
	}
}

func (c *TerminationCompensator) Start(ctx context.Context) {
	c.logger.Info("终止信号补偿器启动")
	for ctx.Err() == nil {
		startedAt := time.Now()
		if err := c.service.DeliverPending(ctx, c.config.BatchSize); err != nil {
			c.logger.Error("投递终止信号失败", elog.FieldErr(err))
		}
		wait := c.config.MinDuration - time.Since(startedAt)
		if wait <= 0 {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}
