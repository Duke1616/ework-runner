package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Duke1616/ecmdb/internal/execute/internal/domain"
	"github.com/Duke1616/ecmdb/internal/runner"
	"github.com/ecodeclub/mq-api"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const TEMPDIR = "./app"

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
	currentTime := time.Now().Format("20060102150405")

	// 创建以 taskId 和当前时间为名的新目录
	dirName := fmt.Sprintf("%d_%s", taskId, currentTime)
	newDirPath := filepath.Join(TEMPDIR, dirName)

	// 创建目录
	if err := os.MkdirAll(newDirPath, os.ModePerm); err != nil {
		fmt.Printf("创建目录 %s 失败: %v\n", newDirPath, err)
		return
	}

	// 移动临时文件
	if tempFile := cmd.Args[1]; tempFile != "" {
		newFilePath := filepath.Join(newDirPath, dirName+filepath.Ext(tempFile))

		// 移动文件
		if err := os.Rename(tempFile, newFilePath); err != nil {
			fmt.Printf("移动文件 %s 失败: %v\n", tempFile, err)
			return
		}
	}

	if args := cmd.Args[2]; args != "" {
		newVarFilePath := filepath.Join(newDirPath, dirName+filepath.Ext(args)+".args")

		// 将内容写入新文件
		content := []byte(args)
		if err := os.WriteFile(newVarFilePath, content, 0644); err != nil {
			fmt.Printf("写入文件 %s 失败: %v\n", newVarFilePath, err)
			return
		}
	}

	// 移动变量文件
	if varFile := cmd.Args[3]; varFile != "" {
		newVarFilePath := filepath.Join(newDirPath, dirName+filepath.Ext(varFile))
		// 移动文件
		if err := os.Rename(varFile, newVarFilePath); err != nil {
			fmt.Printf("移动文件 %s 失败: %v\n", varFile, err)
			return
		}
	}
}
