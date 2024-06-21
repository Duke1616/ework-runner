//go:build wireinject

package ioc

import (
	"github.com/Duke1616/ecmdb/internal/worker"
	"github.com/google/wire"
)

var BaseSet = wire.NewSet(InitViper, InitMQ)

func InitApp() (*App, error) {
	wire.Build(wire.Struct(new(App), "*"),
		BaseSet,
		InitWebServer,
		InitGinMiddlewares,
		worker.InitModule,
		wire.FieldsOf(new(*worker.Module), "Hdl"),
	)
	return new(App), nil
}
