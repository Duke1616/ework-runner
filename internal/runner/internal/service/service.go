package service

import (
	"context"
	"fmt"
	"github.com/Duke1616/ecmdb/internal/runner/internal/domain"
	"github.com/Duke1616/ecmdb/internal/runner/internal/event"
	"github.com/Duke1616/ecmdb/pkg/registry"
	"github.com/spf13/viper"
	"log/slog"
)

type Service interface {
	Register(ctx context.Context, req domain.Runner) error
}

type service struct {
	producer event.TaskRunnerEventProducer
}

func NewService(producer event.TaskRunnerEventProducer) Service {
	return &service{
		producer: producer,
	}
}

func (s *service) Register(ctx context.Context, req domain.Runner) error {
	var cfg registry.Instance
	if err := viper.UnmarshalKey("worker", &cfg); err != nil {
		panic(fmt.Errorf("unable to decode into struct: %v", err))
	}

	err := s.producer.Produce(ctx, event.TaskRunnerEvent{
		CodebookUid:    req.CodebookUid,
		CodebookSecret: req.CodebookSecret,
		WorkerName:     cfg.Name,
		Name:           req.Name,
		Tags:           req.Tags,
		Action:         event.REGISTER,
	})

	if err != nil {
		slog.Error("注册 Runner 失败",
			slog.Any("name", req.Name),
			slog.Any("error", err),
		)
	}

	return nil
}
