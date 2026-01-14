package ioc

import (
	"context"

	"github.com/Duke1616/ecmdb/internal/execute"
	"github.com/Duke1616/ecmdb/internal/service/scheduler"
	"github.com/gin-gonic/gin"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type App struct {
	Web        *gin.Engine
	WorkerSvc  execute.Service
	EtcdClient *clientv3.Client
}

// Task 调度平台上的长任务 —— 各种补偿任务、消费者等
type Task interface {
	Start(ctx context.Context)
}

type SchedulerApp struct {
	//GRPC      *grpc.Component
	Scheduler *scheduler.Scheduler
	Tasks     []Task
}

func (a *SchedulerApp) StartTasks(ctx context.Context) {
	for _, t := range a.Tasks {
		go func(t Task) {
			t.Start(ctx)
		}(t)
	}
}
