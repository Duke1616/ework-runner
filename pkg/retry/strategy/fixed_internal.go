package strategy

import (
	"sync/atomic"
	"time"
)

var _ Strategy = (*FixedIntervalRetryStrategy)(nil)

// FixedIntervalRetryStrategy 等间隔重试
type FixedIntervalRetryStrategy struct {
	maxRetries int32         // 最大重试次数，如果是 0 或负数，表示无限重试
	interval   time.Duration // 重试间隔时间
	retries    int32         // 当前重试次数
}

func NewFixedIntervalRetryStrategy(interval time.Duration, maxRetries int32) *FixedIntervalRetryStrategy {
	return &FixedIntervalRetryStrategy{
		maxRetries: maxRetries,
		interval:   interval,
	}
}

func (s *FixedIntervalRetryStrategy) NextWithRetries(retries int32) (time.Duration, bool) {
	return s.nextWithRetries(retries)
}

func (s *FixedIntervalRetryStrategy) nextWithRetries(retries int32) (time.Duration, bool) {
	if s.maxRetries <= 0 || retries <= s.maxRetries {
		return s.interval, true
	}
	return 0, false
}

func (s *FixedIntervalRetryStrategy) Next() (time.Duration, bool) {
	retries := atomic.AddInt32(&s.retries, 1)
	return s.nextWithRetries(retries)
}

func (s *FixedIntervalRetryStrategy) Report(_ error) Strategy {
	return s
}
