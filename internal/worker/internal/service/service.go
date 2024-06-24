package service

import (
	"context"
	"github.com/Duke1616/ecmdb/internal/runner"
	"github.com/Duke1616/ecmdb/internal/worker/internal/domain"
	"github.com/Duke1616/ecmdb/internal/worker/internal/event"
	"github.com/ecodeclub/mq-api"
	"log/slog"
)

type Service interface {
	Start(ctx context.Context, req domain.Worker) error
	Stop(ctx context.Context, req domain.Worker) error
}

type service struct {
	producer  event.TaskWorkerEventProducer
	runnerSvc runner.Service
	mq        mq.MQ
}

func NewService(mq mq.MQ, producer event.TaskWorkerEventProducer, runnerSvc runner.Service) Service {
	return &service{
		mq:        mq,
		runnerSvc: runnerSvc,
		producer:  producer,
	}
}

func (s *service) Start(ctx context.Context, req domain.Worker) error {
	err := s.start(ctx, req)
	if err != nil {
		return err
	}

	consumer, err := event.NewRunnerConsumer(s.mq, s.runnerSvc, req.Topic)
	if err != nil {
		return err
	}

	consumer.Start(context.Background())

	return nil
}

func (s *service) Stop(ctx context.Context, req domain.Worker) error {
	consumer, err := event.NewRunnerConsumer(s.mq, s.runnerSvc, req.Topic)
	if err != nil {
		return err
	}

	return consumer.Stop(context.Background())
}

func (s *service) start(ctx context.Context, req domain.Worker) error {
	evt := event.WorkerEvent{
		Name:   req.Name,
		Desc:   req.Desc,
		Topic:  req.Topic,
		Status: event.Status(domain.RUNNING),
	}

	if er := s.producer.Produce(ctx, evt); er != nil {
		slog.Error("连接服务端失败",
			slog.Any("error", er),
			slog.Any("event", evt),
		)
	}

	return nil
}

func (s *service) stop(ctx context.Context, req domain.Worker) error {
	evt := event.WorkerEvent{
		Name:   req.Name,
		Desc:   req.Desc,
		Topic:  req.Topic,
		Status: event.Status(domain.STOPPING),
	}
	if er := s.producer.Produce(ctx, evt); er != nil {
		slog.Error("连接服务端失败",
			slog.Any("error", er),
			slog.Any("event", evt),
		)
	}

	return nil
}
