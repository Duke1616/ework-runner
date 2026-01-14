//go:build unit

package strategy

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewExponentialBackoffRetryStrategy_New(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		initialInterval time.Duration
		maxInterval     time.Duration
		maxRetries      int32
		want            *ExponentialBackoffRetryStrategy
		wantErr         error
	}{
		{
			name:            "no error",
			initialInterval: 2 * time.Second,
			maxInterval:     2 * time.Minute,
			maxRetries:      5,
			want: func() *ExponentialBackoffRetryStrategy {
				s := NewExponentialBackoffRetryStrategy(2*time.Second, 2*time.Minute, 5)
				return s
			}(),
			wantErr: nil,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := NewExponentialBackoffRetryStrategy(tt.initialInterval, tt.maxInterval, tt.maxRetries)
			assert.Equal(t, tt.want, s)
		})
	}
}

func TestExponentialBackoffRetryStrategy_Next(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		ctx      context.Context
		strategy *ExponentialBackoffRetryStrategy

		wantIntervals []time.Duration
	}{
		{
			name: "stop if retries reaches maxRetries",
			ctx:  t.Context(),
			strategy: func() *ExponentialBackoffRetryStrategy {
				return NewExponentialBackoffRetryStrategy(1*time.Second, 10*time.Second, 3)
			}(),
			wantIntervals: []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second},
		},
		{
			name: "initialInterval over maxInterval",
			ctx:  t.Context(),
			strategy: func() *ExponentialBackoffRetryStrategy {
				return NewExponentialBackoffRetryStrategy(1*time.Second, 4*time.Second, 5)
			}(),

			wantIntervals: []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second, 4 * time.Second, 4 * time.Second},
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			intervals := make([]time.Duration, 0)
			for {
				if interval, ok := tt.strategy.Next(); ok {
					intervals = append(intervals, interval)
				} else {
					break
				}
			}
			assert.Equal(t, tt.wantIntervals, intervals)
		})
	}
}

// 指数退避重试策略子测试函数，无限重试
func TestExponentialBackoffRetryStrategy_Next4InfiniteRetry(t *testing.T) {
	t.Parallel()
	t.Run("maxRetries equals 0", func(t *testing.T) {
		t.Parallel()
		testNext4InfiniteRetry(t, 0)
	})

	t.Run("maxRetries equals -1", func(t *testing.T) {
		t.Parallel()
		testNext4InfiniteRetry(t, -1)
	})
}

func ExampleExponentialBackoffRetryStrategy_Next() {
	// 注意，因为在例子里面我们设置初始的重试间隔是 1s，最大重试间隔是 5s
	// 所以在前面四次，重试间隔都是在增长的，每次变为原来的2倍。
	// 在触及到了最大重试间隔之后，就一直以最大重试间隔来重试。
	retry := NewExponentialBackoffRetryStrategy(time.Second, time.Second*5, 10)

	interval, ok := retry.Next()
	for ok {
		fmt.Println(interval)
		interval, ok = retry.Next()
	}
	// Output:
	// 1s
	// 2s
	// 4s
	// 5s
	// 5s
	// 5s
	// 5s
	// 5s
	// 5s
	// 5s
}
