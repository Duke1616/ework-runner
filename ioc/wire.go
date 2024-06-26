//go:build wireinject

package ioc

import (
	"github.com/Duke1616/ecmdb/internal/runner"
	"github.com/Duke1616/ecmdb/internal/worker"
	"github.com/google/wire"
)

var BaseSet = wire.NewSet(InitMQ, InitEtcdClient)

func InitApp() (*App, error) {
	wire.Build(wire.Struct(new(App), "*"),
		BaseSet,
		InitWebServer,
		InitGinMiddlewares,
		runner.InitModule,
		wire.FieldsOf(new(*worker.Module), "Svc"),
		worker.InitModule,
		wire.FieldsOf(new(*worker.Module), "Hdl"),
	)
	return new(App), nil
}
