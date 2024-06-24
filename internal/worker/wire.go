//go:build wireinject

package worker

import (
	"github.com/Duke1616/ecmdb/internal/runner"
	"github.com/Duke1616/ecmdb/internal/worker/internal/event"
	"github.com/Duke1616/ecmdb/internal/worker/internal/service"
	"github.com/Duke1616/ecmdb/internal/worker/internal/web"
	"github.com/ecodeclub/mq-api"
	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(
	service.NewService,
	web.NewHandler)

func InitModule(q mq.MQ, runnerSvc *runner.Module) (*Module, error) {
	wire.Build(
		ProviderSet,
		event.NewTaskWorkerEventProducer,
		wire.FieldsOf(new(*runner.Module), "Svc"),
		wire.Struct(new(Module), "*"),
	)
	return new(Module), nil
}
