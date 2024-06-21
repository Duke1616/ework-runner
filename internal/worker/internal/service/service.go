package service

import (
	"context"
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
	producer event.TaskWorkerEventProducer
	mq       mq.MQ
}

func NewService(mq mq.MQ, producer event.TaskWorkerEventProducer) Service {
	return &service{
		mq:       mq,
		producer: producer,
	}
}

func (s *service) Start(ctx context.Context, req domain.Worker) error {
	return s.register(ctx, req)

	// TODO 开启消息队列监听
}

func (s *service) Stop(ctx context.Context, req domain.Worker) error {
	// TODO 关闭消息队列监听
	panic("implement me")
}

func (s *service) register(ctx context.Context, req domain.Worker) error {
	evt := event.WorkerEvent{
		Name:   req.Name,
		Desc:   req.Desc,
		Topic:  req.Topic,
		Status: event.Status(req.Status),
	}

	if er := s.producer.Produce(ctx, evt); er != nil {
		slog.Error("连接服务端失败",
			slog.Any("error", er),
			slog.Any("event", evt),
		)
	}

	return nil
}
