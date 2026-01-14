//go:build unit

package strategy

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFixedIntervalRetryStrategy_Next(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		ctx      context.Context
		s        *FixedIntervalRetryStrategy
		interval time.Duration

		isContinue bool
	}{
		{
			name: "init case, retries 0",
			ctx:  t.Context(),
			s: &FixedIntervalRetryStrategy{
				maxRetries: 3,
				interval:   time.Second,
			},
			interval:   time.Second,
			isContinue: true,
		},
		{
			name: "retries equals to MaxRetries 3 after the increase",
			ctx:  t.Context(),
			s: &FixedIntervalRetryStrategy{
				maxRetries: 3,
				interval:   time.Second,
				retries:    2,
			},
			interval:   time.Second,
			isContinue: true,
		},
		{
			name: "retries over MaxRetries after the increase",
			ctx:  t.Context(),
			s: &FixedIntervalRetryStrategy{
				maxRetries: 3,
				interval:   time.Second,
				retries:    3,
			},
			interval:   0,
			isContinue: false,
		},
		{
			name: "MaxRetries equals to 0",
			ctx:  t.Context(),
			s: &FixedIntervalRetryStrategy{
				maxRetries: 0,
				interval:   time.Second,
			},
			interval:   time.Second,
			isContinue: true,
		},
		{
			name: "negative MaxRetries",
			ctx:  t.Context(),
			s: &FixedIntervalRetryStrategy{
				maxRetries: -1,
				interval:   time.Second,
				retries:    0,
			},
			interval:   time.Second,
			isContinue: true,
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			interval, isContinue := tt.s.Next()
			assert.Equal(t, tt.interval, interval)
			assert.Equal(t, tt.isContinue, isContinue)
		})
	}
}

func TestFixedIntervalRetryStrategy_New(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		maxRetries int32
		interval   time.Duration

		want    *FixedIntervalRetryStrategy
		wantErr error
	}{
		{
			name:       "no error",
			maxRetries: 5,
			interval:   time.Second,

			want: &FixedIntervalRetryStrategy{
				maxRetries: 5,
				interval:   time.Second,
			},
			wantErr: nil,
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := NewFixedIntervalRetryStrategy(tt.interval, tt.maxRetries)
			assert.Equal(t, tt.want, got)
		})
	}
}

func testNext4InfiniteRetry(t *testing.T, maxRetries int32) {
	t.Helper()
	n := 100

	s := NewExponentialBackoffRetryStrategy(1*time.Second, 4*time.Second, maxRetries)

	wantIntervals := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}
	length := n - len(wantIntervals)
	for i := 0; i < length; i++ {
		wantIntervals = append(wantIntervals, 4*time.Second)
	}

	intervals := make([]time.Duration, 0, n)
	for i := 0; i < n; i++ {
		res, _ := s.Next()
		intervals = append(intervals, res)
	}
	assert.Equal(t, wantIntervals, intervals)
}

func ExampleFixedIntervalRetryStrategy_Next() {
	retry := NewFixedIntervalRetryStrategy(time.Second, 3)

	interval, ok := retry.Next()
	for ok {
		fmt.Println(interval)
		interval, ok = retry.Next()
	}
	// Output:
	// 1s
	// 1s
	// 1s
}
