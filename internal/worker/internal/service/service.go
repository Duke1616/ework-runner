package service

import (
	"context"
	"github.com/Duke1616/ecmdb/internal/runner"
	"github.com/Duke1616/ecmdb/internal/worker/internal/domain"
	"github.com/ecodeclub/mq-api"
)

type Service interface {
	Receive(ctx context.Context, req domain.Message) error
}

type service struct {
	runnerSvc runner.Service
	mq        mq.MQ
}

func NewService(mq mq.MQ, runnerSvc runner.Service) Service {
	return &service{
		mq:        mq,
		runnerSvc: runnerSvc,
	}
}

func (s *service) Receive(ctx context.Context, req domain.Message) error {
	err := s.runnerSvc.Start(ctx, runner.Runner{
		Name:     req.Name,
		Language: req.Language,
		Code:     req.Code,
		UUID:     req.UUID,
	})
	if err != nil {
		return err
	}

	return nil
}
