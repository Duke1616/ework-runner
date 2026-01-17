package scripts

import (
	"os/exec"

	"github.com/Duke1616/ework-runner/sdk/executor"
)

// PythonTaskHandler Python 任务处理器
type PythonTaskHandler struct {
	executor *ScriptExecutor
}

func NewPythonTaskHandler() *PythonTaskHandler {
	return &PythonTaskHandler{
		executor: NewScriptExecutor(
			"python",
			"scripts-*.py",
			createPythonCmd,
			passThroughVars,
		),
	}
}

func (h *PythonTaskHandler) Name() string {
	return "python"
}

func (h *PythonTaskHandler) Run(ctx *executor.Context) error {
	return h.executor.Run(ctx)
}

// ---------------------------
// Python 特定逻辑
// ---------------------------

func createPythonCmd(codeFile, args, varsContent string) (*exec.Cmd, error) {
	return exec.Command("python", codeFile, args, varsContent), nil
}

// passThroughVars 直接透传变量字符串 (Python 直接解析 JSON)
func passThroughVars(varsJSON string) (string, error) {
	return varsJSON, nil
}
