package ioc

import (
	"github.com/Duke1616/ecmdb/internal/worker"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

type App struct {
	Web       *gin.Engine
	WorkerSvc worker.Service
	Viper     *viper.Viper
}
