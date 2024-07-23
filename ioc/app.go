package ioc

import (
	"github.com/Duke1616/ecmdb/internal/execute"
	"github.com/gin-gonic/gin"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type App struct {
	Web        *gin.Engine
	WorkerSvc  execute.Service
	EtcdClient *clientv3.Client
}
