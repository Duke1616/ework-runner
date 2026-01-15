//go:build wireinject

package ioc

import (
	"github.com/Duke1616/ework-runner/internal/execute"
	"github.com/Duke1616/ework-runner/internal/runner"
	"github.com/google/wire"
)

var BaseSet = wire.NewSet(InitMQ, InitEtcdClient)

func InitApp() (*App, error) {
	wire.Build(wire.Struct(new(App), "*"),
		BaseSet,
		InitWebServer,
		InitGinMiddlewares,
		runner.InitModule,
		wire.FieldsOf(new(*execute.Module), "Svc", "Hdl"),
		execute.InitModule,
		wire.FieldsOf(new(*runner.Module), "Hdl"),
	)
	return new(App), nil
}
