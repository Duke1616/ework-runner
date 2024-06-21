//go:build wireinject

package worker

import (
	"github.com/Duke1616/ecmdb/internal/worker/internal/event"
	"github.com/Duke1616/ecmdb/internal/worker/internal/service"
	"github.com/Duke1616/ecmdb/internal/worker/internal/web"
	"github.com/ecodeclub/mq-api"
	"github.com/google/wire"
)

func InitModule(q mq.MQ) (*Module, error) {
	wire.Build(
		event.NewTaskWorkerEventProducer,
		service.NewService,
		web.NewHandler,
		wire.Struct(new(Module), "*"),
	)
	return new(Module), nil
}

//var (
//	taskOnce = sync.Once{}
//	svc      Service
//)
//
//func initRegister(q mq.MQ, p event.TaskWorkerEventProducer, viper *viper.Viper) service.Service {
//	taskOnce.Do(func() {
//		type Config struct {
//			Name  string `yaml:"name"`
//			Desc  string `yaml:"desc"`
//			Topic string `yaml:"topic"`
//		}
//
//		var cfg Config
//		if err := viper.UnmarshalKey("worker", &cfg); err != nil {
//			panic(fmt.Errorf("unable to decode into struct: %v", err))
//		}
//		worker := Worker{
//			Name:  cfg.Name,
//			Desc:  cfg.Desc,
//			Topic: cfg.Topic,
//		}
//
//		svc = service.NewService(q, p)
//		svc.Register(context.Background(), worker)
//	})
//
//	return svc
//}
