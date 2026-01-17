package grpc

import (
	"fmt"
	"time"

	"github.com/Duke1616/ework-runner/sdk/executor"
	"github.com/gotomicro/ego/core/elog"
)

// DemoTaskHandler 演示任务处理器
// NOTE: 这是一个示例实现,展示如何使用 SDK 处理带进度的任务
func DemoTaskHandler(ctx *executor.Context) error {
	logger := ctx.Logger()

	// 获取参数
	start := ctx.ParamInt("start")
	end := ctx.ParamInt("end")

	if end <= 0 {
		return fmt.Errorf("invalid end value: %d", end)
	}

	logger.Info("开始执行任务",
		elog.Int("start", start),
		elog.Int("end", end))

	total := end
	progressUnits := start

	fmt.Println("开始调用任务", elog.Int("progressUnits", progressUnits))

	// 模拟任务执行,定期更新进度
	incTicker := time.NewTicker(100 * time.Millisecond)
	defer incTicker.Stop()

	for progressUnits < total {
		select {
		case <-incTicker.C:
			progressUnits++
			progress := progressUnits * 100 / total

			// 上报进度 (可选)
			if err := ctx.ReportProgress(progress); err != nil {
				logger.Error("上报进度失败", elog.FieldErr(err))
			}

			if progressUnits%1000 == 0 {
				logger.Info("任务进度",
					elog.Int("current", progressUnits),
					elog.Int("total", total),
					elog.Int("progress", progress))
			}
		}
	}

	logger.Info("任务执行完成",
		elog.Int("processed", progressUnits))

	return nil
}
