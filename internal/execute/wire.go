//go:build wireinject

package execute

import (
	"context"
	"github.com/Duke1616/ecmdb/internal/execute/internal/event"
	"github.com/Duke1616/ecmdb/internal/execute/internal/service"
	"github.com/Duke1616/ecmdb/internal/execute/internal/web"
	"github.com/Duke1616/ecmdb/internal/runner"
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
		initExecuteConsumer,
		event.NewExecuteResultEventProducer,
		wire.FieldsOf(new(*runner.Module), "Svc"),
		wire.Struct(new(Module), "*"),
	)
	return new(Module), nil
}

func initExecuteConsumer(q mq.MQ, svc service.Service, producer event.TaskExecuteResultProducer) *event.ExecuteConsumer {
	var cfg registry.Instance
	err := viper.UnmarshalKey("worker", &cfg)

	consumer, err := event.NewExecuteConsumer(q, svc, cfg.Topic, producer)
	if err != nil {
		panic(err)
	}

	consumer.Start(context.Background())
	return consumer
}
