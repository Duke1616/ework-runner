package ioc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWaitForMigrationLock(t *testing.T) {
	testCases := []struct {
		name      string
		acquire   func(context.Context) (bool, error)
		wantCalls int
		wantErr   error
	}{
		{
			name:      "立即获取",
			acquire:   func(context.Context) (bool, error) { return true, nil },
			wantCalls: 1,
		},
		{
			name: "轮询后获取",
			acquire: func() func(context.Context) (bool, error) {
				calls := 0
				return func(context.Context) (bool, error) {
					calls++
					return calls == 3, nil
				}
			}(),
			wantCalls: 3,
		},
		{
			name:      "查询失败",
			acquire:   func(context.Context) (bool, error) { return false, errors.New("database unavailable") },
			wantCalls: 1,
			wantErr:   errors.New("database unavailable"),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			calls := 0
			err := waitForMigrationLock(context.Background(), time.Second, time.Millisecond,
				func(ctx context.Context) (bool, error) {
					calls++
					return testCase.acquire(ctx)
				})

			require.Equal(t, testCase.wantCalls, calls)
			if testCase.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, testCase.wantErr.Error())
		})
	}
}

func TestWaitForMigrationLockTimeout(t *testing.T) {
	err := waitForMigrationLock(context.Background(), 5*time.Millisecond, time.Millisecond,
		func(context.Context) (bool, error) { return false, nil })

	require.ErrorIs(t, err, errMigrationLockTimeout)
}

func TestWaitForMigrationLockCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := waitForMigrationLock(ctx, time.Second, time.Millisecond,
		func(context.Context) (bool, error) { return false, nil })

	require.ErrorIs(t, err, context.Canceled)
}
