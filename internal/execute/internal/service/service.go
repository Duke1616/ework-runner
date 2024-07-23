package service

import (
	"bufio"
	"context"
	"fmt"
	"github.com/Duke1616/ecmdb/internal/execute/internal/domain"
	"github.com/Duke1616/ecmdb/internal/runner"
	"github.com/ecodeclub/mq-api"
	"io"
	"os/exec"
	"sync"
)

type Service interface {
	Receive(ctx context.Context, req domain.ExecuteReceive) (string, domain.Status, error)
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

func (s *service) Receive(ctx context.Context, req domain.ExecuteReceive) (string, domain.Status, error) {
	return s.combined(isLanguage(req.Language, req.Code))
}

func isLanguage(language string, code string) *exec.Cmd {
	var cmd *exec.Cmd
	switch language {
	case "shell":
		shell := "/bin/bash"
		if _, err := exec.LookPath(shell); err != nil {
			shell = "/bin/sh"
		}

		if language == "shell" {
			cmd = exec.Command(shell, "-c", code)
		} else {
			// 执行其他语言的脚本
			cmd = exec.Command(language, code)
		}
	case "python":

	}

	return cmd
}

func (s *service) combined(cmd *exec.Cmd) (string, domain.Status, error) {
	// 运行命令并获取输出
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), domain.FAILED, err
	}

	return string(output), domain.SUCCESS, err
}

// 实时输出
func stdoutPipe(cmd *exec.Cmd) error {
	var wg sync.WaitGroup
	wg.Add(2)
	//捕获标准输出
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	readout := bufio.NewReader(stdout)
	go func() {
		defer wg.Done()
		getOutput(readout)
	}()

	//捕获标准错误
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	reader := bufio.NewReader(stderr)
	go func() {
		defer wg.Done()
		getOutput(reader)
	}()

	//执行命令
	if err = cmd.Run(); err != nil {
		return err
	}

	wg.Wait()

	return nil
}

func getOutput(reader *bufio.Reader) {
	var sumOutput string
	outputBytes := make([]byte, 200)
	for {
		n, err := reader.Read(outputBytes)
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Println(err)
			sumOutput += err.Error()
		}
		output := string(outputBytes[:n])
		fmt.Print(output)
		sumOutput += output
	}
	return
}
