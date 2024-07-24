package service

import (
	"bufio"
	"context"
	"fmt"
	"github.com/Duke1616/ecmdb/internal/execute/internal/domain"
	"github.com/Duke1616/ecmdb/internal/runner"
	"github.com/ecodeclub/mq-api"
	"io"
	"os"
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
	return s.combined(isLanguage(req.Language, req.Code, req.Args))
}

func isLanguage(language string, code string, args string) *exec.Cmd {
	var cmd *exec.Cmd
	// 创建临时文件
	tempFile := createTempFile(code)
	defer os.Remove(tempFile)

	// 判断语言处理逻辑
	switch language {
	case "shell":

		// 判断系统是否有bash、如果没有降级为sh
		shell := "/bin/bash"
		if _, err := exec.LookPath(shell); err != nil {
			shell = "/bin/sh"
		}

		// 执行指令
		cmd = exec.Command(shell, "-c", tempFile, args)
	case "python":

	}

	return cmd
}

func createTempFile(code string) string {
	// 创建临时文件
	tmpFile, err := os.CreateTemp("", "scripts-*.sh")
	if err != nil {
		fmt.Println("Error creating temporary file:", err)
	}

	// 写入临时文件
	content := []byte(code)
	if _, err = tmpFile.Write(content); err != nil {
		fmt.Println("Error writing to temporary file:", err)
	}

	// 关闭临时文件
	if err = tmpFile.Close(); err != nil {
		fmt.Println("Error closing temporary file:", err)
	}

	// 查看临时文件
	fmt.Println("Temporary file created:", tmpFile.Name())

	return tmpFile.Name()
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
