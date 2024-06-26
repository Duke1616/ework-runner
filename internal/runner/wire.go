//go:build wireinject

package runner

import (
	"github.com/Duke1616/ecmdb/internal/runner/internal/event"
	"github.com/Duke1616/ecmdb/internal/runner/internal/service"
	"github.com/Duke1616/ecmdb/internal/runner/internal/web"
	"github.com/ecodeclub/mq-api"
	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(
	service.NewService,
	web.NewHandler)

func InitModule(q mq.MQ) (*Module, error) {
	wire.Build(
		ProviderSet,
		event.NewTaskRunnerEventProducer,
		wire.Struct(new(Module), "*"),
	)
	return new(Module), nil
}
