package service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/Duke1616/ecmdb/internal/execute/internal/domain"
	"github.com/Duke1616/ecmdb/internal/runner"
	"github.com/ecodeclub/mq-api"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

const TEMPDIR = "/app"

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
	return s.combined(isLanguage(req.Language, req.Code, req.Args, req.Variables), req.TaskId)
}

func isLanguage(language, code, args, variables string) *exec.Cmd {
	var cmd *exec.Cmd
	// 创建临时文件
	codeFile := createCodeTempFile(code, language)

	// 变量临时文件
	varsFile := createVariablesTempFile(variables)

	// 判断语言处理逻辑
	switch language {
	case "shell":
		// 判断系统是否有bash、如果没有降级为sh
		shell := "/bin/bash"
		if _, err := exec.LookPath(shell); err != nil {
			shell = "/bin/sh"
		}

		// 执行指令
		cmd = exec.Command(shell, codeFile, args, varsFile)
	case "python":
		cmd = exec.Command("python", codeFile, args, variables)
	}

	return cmd
}

func createVariablesTempFile(vars string) string {
	// 用于存储解析后的 JSON 数据
	var variables []domain.Variable

	// 解析 JSON 数据
	err := json.Unmarshal([]byte(vars), &variables)
	if err != nil {
		slog.Error("unmarshal error:", slog.Any("错误信息", err))
	}

	// 打开文件用于写入
	tmpFile, err := os.CreateTemp(TEMPDIR, "scripts-*.vars")
	if err != nil {
		slog.Error("creating temporary file:", slog.Any("错误信息", err))
	}

	// 遍历数据并写入文件
	for _, item := range variables {
		_, err = tmpFile.WriteString(fmt.Sprintf("%s=%s\n", item.Key, item.Value))
		if err != nil {
			slog.Error("writing to temporary file:", slog.Any("错误信息", err))
		}
	}

	// 关闭临时文件
	if err = tmpFile.Close(); err != nil {
		slog.Error("closing temporary file:", slog.Any("错误信息", err))
	}

	// 设置临时文件权限为可执行
	if err = os.Chmod(tmpFile.Name(), 0700); err != nil {
		slog.Error("setting temporary file permissions:", slog.Any("错误信息", err))
	}

	return tmpFile.Name()
}

func createCodeTempFile(code string, language string) string {
	// 判断文件后缀
	fileName := ""
	switch language {
	case "shell":
		fileName = "scripts-*.sh"
	case "python":
		fileName = "scripts-*.py"
	}

	// 创建临时文件
	tmpFile, err := os.CreateTemp(TEMPDIR, fileName)
	if err != nil {
		slog.Error("creating temporary file:", slog.Any("错误信息", err))
	}

	// 写入临时文件
	content := []byte(code)
	if _, err = tmpFile.Write(content); err != nil {
		slog.Error("writing to temporary file:", slog.Any("错误信息", err))
	}

	// 关闭临时文件
	if err = tmpFile.Close(); err != nil {
		slog.Error("closing temporary file:", slog.Any("错误信息", err))
	}

	// 设置临时文件权限为可执行
	if err = os.Chmod(tmpFile.Name(), 0700); err != nil {
		slog.Error("setting temporary file permissions:", slog.Any("错误信息", err))
	}

	return tmpFile.Name()
}

func (s *service) combined(cmd *exec.Cmd, taskId int64) (string, domain.Status, error) {
	// 运行命令并获取输出
	output, err := cmd.CombinedOutput()
	if err != nil {
		moveTempFile(cmd, taskId)
		return string(output), domain.FAILED, err
	}

	moveTempFile(cmd, taskId)
	return string(output), domain.SUCCESS, err
}

func moveTempFile(cmd *exec.Cmd, taskId int64) {
	// 获取当前时间到秒
	currentTime := time.Now().Format("20060102_150405")

	// 拼接新的文件名
	newFileName := fmt.Sprintf("%d_%s", taskId, currentTime)

	// 获取临时文件路径并移动
	if tempFile := cmd.Args[1]; tempFile != "" {
		// 新的目标文件路径
		newFilePath := filepath.Join(TEMPDIR, newFileName+filepath.Ext(tempFile))

		// 移动文件
		err := os.Rename(tempFile, newFilePath)
		if err != nil {
			fmt.Printf("移动文件 %s 失败: %v\n", tempFile, err)
			return
		}
	}

	if varFile := cmd.Args[2]; varFile != "" {
		// 新的目标文件路径
		newVarFilePath := filepath.Join(TEMPDIR, newFileName+filepath.Ext(varFile))

		// 移动文件
		err := os.Rename(varFile, newVarFilePath)
		if err != nil {
			fmt.Printf("移动文件 %s 失败: %v\n", varFile, err)
			return
		}
	}
}

func removeTempFile(cmd *exec.Cmd) {
	// 删除临时文件
	if tempFile := cmd.Args[1]; tempFile != "" {
		err := os.Remove(tempFile)
		if err != nil {
			return
		}
	}

	if varFile := cmd.Args[3]; varFile != "" {
		err := os.Remove(varFile)
		if err != nil {
			return
		}
	}
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
