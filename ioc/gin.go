package ioc

import (
	"time"

	"github.com/Duke1616/ework-runner/internal/execute"
	"github.com/Duke1616/ework-runner/internal/runner"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func InitWebServer(mdls []gin.HandlerFunc, workerHdl *execute.Handler, runnerHdl *runner.Handler) *gin.Engine {
	server := gin.Default()

	server.Use(mdls...)

	workerHdl.RegisterRoutes(server)
	runnerHdl.RegisterRoutes(server)
	return server
}

func InitGinMiddlewares() []gin.HandlerFunc {
	return []gin.HandlerFunc{
		corsHdl(),
		func(ctx *gin.Context) {
		},
	}
}

func corsHdl() gin.HandlerFunc {
	return cors.New(cors.Config{
		AllowOrigins:  []string{"*"},
		AllowMethods:  []string{"POST", "GET", "PUT", "DELETE", "PATCH", "OPTIONS"},
		AllowHeaders:  []string{"Content-Type", "Authorization"},
		ExposeHeaders: []string{"X-Access-Token"},
		// 是否允许你带 cookie 之类的东西
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	})
}
