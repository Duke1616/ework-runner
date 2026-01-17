package scripts

import (
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/Duke1616/ework-runner/sdk/executor"
)

// ShellTaskHandler Shell 任务处理器
type ShellTaskHandler struct {
	executor *ScriptExecutor
}

func NewShellTaskHandler() *ShellTaskHandler {
	return &ShellTaskHandler{
		executor: NewScriptExecutor(
			"shell",
			"scripts-*.sh",
			createShellCmd,
			prepareShellVars,
		),
	}
}

func (h *ShellTaskHandler) Name() string {
	return "shell"
}

func (h *ShellTaskHandler) Run(ctx *executor.Context) error {
	return h.executor.Run(ctx)
}

// ---------------------------
// Shell 特定逻辑
// ---------------------------

func createShellCmd(codeFile, args, varsFile string) (*exec.Cmd, error) {
	shell := "/bin/bash"
	if _, err := exec.LookPath(shell); err != nil {
		shell = "/bin/sh"
	}
	return exec.Command(shell, codeFile, args, varsFile), nil
}

// prepareShellVars 将 JSON 变量转换为 KEY=VALUE 格式的临时文件
func prepareShellVars(varsJSON string) (string, error) {
	if varsJSON == "" {
		return "", nil
	}

	// 解析 JSON
	var variables []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal([]byte(varsJSON), &variables); err != nil {
		return "", err
	}

	// 转换为 shell 变量格式 KEY=VALUE
	var content string
	for _, v := range variables {
		content += fmt.Sprintf("%s=%s\n", v.Key, v.Value)
	}

	// 创建变量文件
	return createTempFile("scripts-*.vars", []byte(content))
}
