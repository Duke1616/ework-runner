package service

import (
	"context"
	"fmt"
	"github.com/Duke1616/ecmdb/internal/runner"
	"github.com/Duke1616/ecmdb/internal/worker/internal/domain"
	"github.com/ecodeclub/mq-api"
	"os/exec"
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
	err := s.start(ctx, req)
	if err != nil {
		return err
	}

	return nil
}

func (s *service) start(ctx context.Context, req domain.Message) error {
	shell := "/bin/bash"
	if _, err := exec.LookPath(shell); err != nil {
		shell = "/bin/sh"
	}

	var cmd *exec.Cmd
	if req.Language == "shell" {
		cmd = exec.Command(shell, "-c", req.Code)
	} else {
		// 执行其他语言的脚本
		cmd = exec.Command(req.Language, req.Code)
	}

	// 运行命令并获取输出
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("命令执行出错: %s\n", err)
		return err
	}

	// 打印命令输出
	fmt.Printf("脚本输出: %s\n", output)

	return nil
}
