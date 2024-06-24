package service

import (
	"context"
	"fmt"
	"github.com/Duke1616/ecmdb/internal/runner/internal/domain"
	"os/exec"
)

type Service interface {
	Start(ctx context.Context, req domain.Runner) error
}

type service struct {
}

func NewService() Service {
	return &service{}
}

func (s *service) Start(ctx context.Context, req domain.Runner) error {
	var cmd *exec.Cmd
	if req.Language == "shell" {
		cmd = exec.Command("/bin/bash", "-c", req.Code)
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
