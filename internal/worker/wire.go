//go:build wireinject

package worker

import (
	"context"
	"github.com/Duke1616/ecmdb/internal/runner"
	"github.com/Duke1616/ecmdb/internal/worker/internal/event"
	"github.com/Duke1616/ecmdb/internal/worker/internal/service"
	"github.com/Duke1616/ecmdb/internal/worker/internal/web"
	"github.com/Duke1616/ecmdb/pkg/registry"
	"github.com/ecodeclub/mq-api"
	"github.com/google/wire"
	"github.com/spf13/viper"
)

var ProviderSet = wire.NewSet(
	service.NewService,
	web.NewHandler)

func InitModule(q mq.MQ, runnerSvc *runner.Module) (*Module, error) {
	wire.Build(
		ProviderSet,
		initWorkerConsumer,
		wire.FieldsOf(new(*runner.Module), "Svc"),
		wire.Struct(new(Module), "*"),
	)
	return new(Module), nil
}

func initWorkerConsumer(q mq.MQ, svc service.Service) *event.WorkerConsumer {
	var cfg registry.Instance
	err := viper.UnmarshalKey("worker", &cfg)

	consumer, err := event.NewWorkerConsumer(q, svc, cfg.Topic)
	if err != nil {
		panic(err)
	}

	consumer.Start(context.Background())
	return consumer
}
