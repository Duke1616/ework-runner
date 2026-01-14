package main

import (
	"context"

	"github.com/Duke1616/ecmdb/cmd/scheduler/ioc"
	"github.com/gotomicro/ego"
	"github.com/gotomicro/ego/core/elog"
	"github.com/gotomicro/ego/server"
	"github.com/gotomicro/ego/server/egovernor"
)

func main() {
	// 创建 ego 应用实例
	egoApp := ego.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app := ioc.InitSchedulerApp()
	app.StartTasks(ctx)

	// 启动服务
	if err := egoApp.Serve(
		egovernor.Load("server.governor").Build(),
		//func() server.Server {
		//	return app.GRPC
		//}(),
		func() server.Server {
			return app.Scheduler
		}(),
	).Cron().
		Run(); err != nil {
		elog.Panic("startup", elog.FieldErr(err))
	}
}
