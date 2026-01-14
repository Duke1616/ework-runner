package strategy

import (
	"math"
	"sync/atomic"
	"time"
)

var _ Strategy = (*ExponentialBackoffRetryStrategy)(nil)

// ExponentialBackoffRetryStrategy 指数退避重试
type ExponentialBackoffRetryStrategy struct {
	// 初始重试间隔
	initialInterval time.Duration
	// 最大重试间隔
	maxInterval time.Duration
	// 最大重试次数
	maxRetries int32
	// 当前重试次数
	retries int32
	// 是否已经达到最大重试间隔
	maxIntervalReached atomic.Value
}

func NewExponentialBackoffRetryStrategy(initialInterval, maxInterval time.Duration, maxRetries int32) *ExponentialBackoffRetryStrategy {
	return &ExponentialBackoffRetryStrategy{
		initialInterval: initialInterval,
		maxInterval:     maxInterval,
		maxRetries:      maxRetries,
	}
}

func (s *ExponentialBackoffRetryStrategy) Report(_ error) Strategy {
	return s
}

func (s *ExponentialBackoffRetryStrategy) NextWithRetries(retries int32) (time.Duration, bool) {
	return s.nextWithRetries(retries)
}

func (s *ExponentialBackoffRetryStrategy) nextWithRetries(retries int32) (time.Duration, bool) {
	if s.maxRetries <= 0 || retries <= s.maxRetries {
		if reached, ok := s.maxIntervalReached.Load().(bool); ok && reached {
			return s.maxInterval, true
		}
		const two = 2
		interval := s.initialInterval * time.Duration(math.Pow(two, float64(retries-1)))
		// 溢出或当前重试间隔大于最大重试间隔
		if interval <= 0 || interval > s.maxInterval {
			s.maxIntervalReached.Store(true)
			return s.maxInterval, true
		}
		return interval, true
	}
	return 0, false
}

func (s *ExponentialBackoffRetryStrategy) Next() (time.Duration, bool) {
	retries := atomic.AddInt32(&s.retries, 1)
	return s.nextWithRetries(retries)
}
