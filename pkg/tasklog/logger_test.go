package tasklog_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Duke1616/etask/pkg/tasklog"
	tasklogmocks "github.com/Duke1616/etask/pkg/tasklog/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestLoggerFlushesByCapacity(t *testing.T) {
	ctrl := gomock.NewController(t)
	sink := tasklogmocks.NewMockSink(ctrl)
	written := make(chan struct{}, 1)
	sink.EXPECT().WriteBatch(gomock.Any(), []string{"first", "second"}).
		DoAndReturn(func(context.Context, []string) error {
			written <- struct{}{}
			return nil
		})

	logger := tasklog.New(context.Background(), sink, tasklog.Options{
		BufferSize: 2, FlushPeriod: time.Hour,
	})
	logger.Log("first")
	logger.Log("second")
	select {
	case <-written:
	case <-time.After(time.Second):
		t.Fatal("日志达到容量后没有及时刷新")
	}
	logger.Close()
	require.Empty(t, logger.PendingLogs())
}

func TestLoggerKeepsFailedBatchInOrder(t *testing.T) {
	ctrl := gomock.NewController(t)
	sink := tasklogmocks.NewMockSink(ctrl)
	written := make(chan struct{}, 4)
	sink.EXPECT().WriteBatch(gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(context.Context, []string) error {
			written <- struct{}{}
			return errors.New("发送失败")
		})

	logger := tasklog.New(context.Background(), sink, tasklog.Options{
		BufferSize: 1, FlushPeriod: time.Hour,
	})
	logger.Log("first")
	select {
	case <-written:
	case <-time.After(time.Second):
		t.Fatal("失败批次没有触发刷新")
	}
	deadline := time.Now().Add(time.Second)
	for len(logger.PendingLogs()) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	logger.Log("second")
	logger.Close()
	require.Equal(t, []string{"first", "second"}, logger.PendingLogs())
}
