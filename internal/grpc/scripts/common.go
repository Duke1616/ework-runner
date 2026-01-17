
package scripts

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/Duke1616/ework-runner/sdk/executor"
	"github.com/gotomicro/ego/core/elog"
)

const TEMPDIR = "/app"

// ---------------------------
// 通用抽象定义
// ---------------------------

// CmdBuilder 构建命令行的函数签名
type CmdBuilder func(codeFile string, args string, varsResource string) (*exec.Cmd, error)

// VarsProcessor 处理变量的函数签名
// 返回的 string 可以是文件路径(Shell) 或 原始内容(Python)
type VarsProcessor func(varsJSON string) (string, error)

// ScriptExecutor 通用脚本执行器
type ScriptExecutor struct {
	language      string
	filePattern   string
	cmdBuilder    CmdBuilder
	varsProcessor VarsProcessor
}

func NewScriptExecutor(
	language string,
	filePattern string,
	cmdBuilder CmdBuilder,
	varsProcessor VarsProcessor,
) *ScriptExecutor {
	return &ScriptExecutor{
		language:      language,
		filePattern:   filePattern,
		cmdBuilder:    cmdBuilder,
		varsProcessor: varsProcessor,
	}
}

func (e *ScriptExecutor) Run(ctx *executor.Context) error {
	logger := ctx.Logger()

	// 1. 获取参数
	code := ctx.Param("code")
	args := ctx.Param("args")
	vars := ctx.Param("variables")

	if code == "" {
		return fmt.Errorf("[%s] code parameter is required", e.language)
	}

	// 2. 准备执行环境
	// 创建代码临时文件
	codeFile, err := createTempFile(e.filePattern, []byte(code))
	if err != nil {
		return fmt.Errorf("create code file failed: %w", err)
	}

	// 处理变量 (部分语言需要转为文件,部分直接传参)
	varsResource, err := e.varsProcessor(vars)
	if err != nil {
		return fmt.Errorf("process vars failed: %w", err)
	}

	// 3. 构建命令
	cmd, err := e.cmdBuilder(codeFile, args, varsResource)
	if err != nil {
		return fmt.Errorf("create cmd failed: %w", err)
	}

	defer e.archive(ctx.TaskID, codeFile, args, vars, varsResource)

	// 4. 执行命令
	logger.Info("开始执行脚本", elog.String("language", e.language))
	output, err := cmd.CombinedOutput()
	logger.Info("脚本输出", elog.String("output", string(output)))

	if err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}
	return nil
}

// archive 归档执行现场
func (e *ScriptExecutor) archive(taskID int64, codeFile string, args string, rawVars string, varsResource string) {
	// 创建归档目录
	currentTime := time.Now().Format("20060102150405")
	dirName := fmt.Sprintf("%d_%s", taskID, currentTime)
	archiveDir := filepath.Join(TEMPDIR, dirName)

	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		fmt.Printf("create archive dir failed: %v\n", err)
		return
	}

	// 1. 归档代码文件
	moveFile(codeFile, archiveDir)

	// 2. 归档参数
	if args != "" {
		saveFile(filepath.Join(archiveDir, "scripts.args"), []byte(args))
	}

	// 3. 归档原始变量 JSON
	if rawVars != "" {
		saveFile(filepath.Join(archiveDir, "scripts.vars.json"), []byte(rawVars))
	}

	// 4. 尝试归档处理后的变量文件 (如果是文件的话)
	// 判断 varsResource 是否为文件路径且存在
	if varsResource != "" && varsResource != rawVars {
		// 简单判断：只有当它是一个存在的文件时才移动
		// 因为 varsProcessor 可能返回原始内容字符串(如Python), 此时不应尝试移动
		if _, err := os.Stat(varsResource); err == nil {
			moveFile(varsResource, archiveDir)
		}
	}
}

// ---------------------------
// 基础 Helper 函数
// ---------------------------
func createTempFile(pattern string, content []byte) (string, error) {
	f, err := os.CreateTemp(TEMPDIR, pattern)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err = f.Write(content); err != nil {
		return "", err
	}

	// 赋予执行权限
	if err = f.Chmod(0700); err != nil {
		return "", err
	}
	return f.Name(), nil
}

func moveFile(src, destDir string) {
	if src == "" {
		return
	}
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return
	}

	base := filepath.Base(src)
	dest := filepath.Join(destDir, "scripts"+filepath.Ext(base))

	if err := os.Rename(src, dest); err != nil {
		fmt.Printf("move file %s failed: %v\n", src, err)
	}
}

func saveFile(path string, content []byte) {
	if err := os.WriteFile(path, content, 0644); err != nil {
		fmt.Printf("save file %s failed: %v\n", path, err)
	}
}
