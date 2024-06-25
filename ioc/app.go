package ioc

import (
	"github.com/Duke1616/ecmdb/internal/worker"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type App struct {
	Web        *gin.Engine
	WorkerSvc  worker.Service
	Viper      *viper.Viper
	EtcdClient *clientv3.Client
}
