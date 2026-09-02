package ioc

import (
	"context"
	"net"
	"time"

	"github.com/Duke1616/eiam/pkg/web/capability"
	"github.com/Duke1616/eiam/pkg/web/capability/syncer"
	"github.com/Duke1616/eiam/pkg/web/middleware"
	"github.com/Duke1616/eiam/pkg/web/sdk"
	artifactWeb "github.com/Duke1616/etask/internal/web/artifact"
	codeassistWeb "github.com/Duke1616/etask/internal/web/codeassist"
	codebookWeb "github.com/Duke1616/etask/internal/web/codebook"
	"github.com/Duke1616/etask/internal/web/manager"
	poolWeb "github.com/Duke1616/etask/internal/web/pool"
	previewWeb "github.com/Duke1616/etask/internal/web/preview"
	resourceWeb "github.com/Duke1616/etask/internal/web/resource"
	runnerWeb "github.com/Duke1616/etask/internal/web/runner"
	variableWeb "github.com/Duke1616/etask/internal/web/variable"
	"github.com/gin-gonic/gin"
	"github.com/gotomicro/ego/core/elog"
	"github.com/gotomicro/ego/server/egin"
)

func InitGinWebServer(mdls []gin.HandlerFunc, sdk *sdk.SDK,
	s syncer.Syncer, providers []capability.PermissionProvider,
	taskHdl *manager.Handler, codebookHdl *codebookWeb.Handler, artifactHdl *artifactWeb.Handler,
	codeassistHdl *codeassistWeb.Handler,
	previewHdl *previewWeb.Handler,
	runnerHdl *runnerWeb.Handler, variableHdl *variableWeb.Handler,
	poolAdminHdl *poolWeb.AdminHandler,
	resourceHdl *resourceWeb.Handler, listener net.Listener) *egin.Component {

	server := egin.Load("server.egin").Build(egin.WithListener(listener))
	// 开启 ContextWithFallback：使 ctx.Context.Value() 自动 fallback 到 ctx.Request.Context().Value()
	server.Engine.ContextWithFallback = true
	server.Use(mdls...)

	// 注册公开路由
	taskHdl.PublicRoutes(server.Engine)
	codebookHdl.PublicRoutes(server.Engine)
	artifactHdl.PublicRoutes(server.Engine)
	codeassistHdl.PublicRoutes(server.Engine)
	previewHdl.PublicRoutes(server.Engine)
	runnerHdl.PublicRoutes(server.Engine)
	variableHdl.PublicRoutes(server.Engine)
	poolAdminHdl.PublicRoutes(server.Engine)
	resourceHdl.PublicRoutes(server.Engine)

	// 登录检查
	server.Use(sdk.CheckLogin())

	// 需要登陆校验的接口
	taskHdl.IdentifyRoutes(server.Engine)
	codebookHdl.IdentifyRoutes(server.Engine)
	artifactHdl.IdentifyRoutes(server.Engine)
	codeassistHdl.IdentifyRoutes(server.Engine)
	previewHdl.IdentifyRoutes(server.Engine)
	runnerHdl.IdentifyRoutes(server.Engine)
	variableHdl.IdentifyRoutes(server.Engine)
	poolAdminHdl.IdentifyRoutes(server.Engine)
	resourceHdl.IdentifyRoutes(server.Engine)

	// 权限策略检查
	server.Use(sdk.CheckPolicy())

	// 注册私有路由
	taskHdl.PrivateRoutes(server.Engine)
	codebookHdl.PrivateRoutes(server.Engine)
	artifactHdl.PrivateRoutes(server.Engine)
	codeassistHdl.PrivateRoutes(server.Engine)
	previewHdl.PrivateRoutes(server.Engine)
	runnerHdl.PrivateRoutes(server.Engine)
	variableHdl.PrivateRoutes(server.Engine)
	poolAdminHdl.PrivateRoutes(server.Engine)
	resourceHdl.PrivateRoutes(server.Engine)

	// 异步启动 EIAM 资产注册控制器
	go func() {
		// 延迟执行，确保路由完全就绪
		time.Sleep(time.Second)

		if err := s.WithOption(
			capability.WithPermissions(providers...),
			capability.WithRouter(server.Engine),
		).Sync(context.Background()); err != nil {
			elog.Error("EIAM 资产注册控制器启动失败", elog.FieldErr(err))
		}
	}()

	return server
}

func InitGinMiddlewares() []gin.HandlerFunc {
	return []gin.HandlerFunc{
		middleware.AccessLogger(),
		middleware.NewCorsBuilder().Build(),
	}
}
