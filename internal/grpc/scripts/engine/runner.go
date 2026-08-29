package engine

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/Duke1616/etask/sdk/executor"
	"github.com/gotomicro/ego/core/elog"
	"github.com/samber/lo"
)

type commandRunner struct {
	maxLogLineSize int
	maxResultSize  int64
	launcher       ProcessLauncher
}

func (r commandRunner) Run(task *executor.Context, workspace Workspace, prepared PreparedCommand) error {
	command := prepared.Command
	if command == nil {
		return fmt.Errorf("语言适配器返回了空命令")
	}
	command.Dir = workspace.ProgramRoot()
	command.Env = MergeEnvironment(workspace.Environment(), prepared.Environment)

	// 子进程通过额外文件描述符 3 输出结构化 JSON，避免与普通日志混在 stdout。
	resultReader, resultWriter, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("创建结果管道失败: %w", err)
	}
	defer resultReader.Close()
	defer resultWriter.Close()
	command.ExtraFiles = []*os.File{resultWriter}

	stdout, err := command.StdoutPipe()
	if err != nil {
		return fmt.Errorf("获取标准输出管道失败: %w", err)
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return fmt.Errorf("获取标准错误管道失败: %w", err)
	}
	if err = r.launcher.Start(command); err != nil {
		return fmt.Errorf("启动执行命令失败: %w", err)
	}
	// 父进程必须关闭写端，读取协程才能在子进程退出后收到 EOF。
	_ = resultWriter.Close()

	// 同时消费三个管道，防止任一输出缓冲区写满后阻塞子进程。
	var wait sync.WaitGroup
	wait.Add(3)
	go streamOutput(task, stdout, r.maxLogLineSize, &wait)
	go streamOutput(task, stderr, r.maxLogLineSize, &wait)
	go streamResult(task, resultReader, r.maxResultSize, &wait)
	err = command.Wait()
	wait.Wait()
	if err != nil {
		if task.Context().Err() != nil {
			return fmt.Errorf("脚本执行被取消或超时: %w", task.Context().Err())
		}
		return fmt.Errorf("脚本执行失败（退出码非 0）: %w", err)
	}
	return nil
}

func streamOutput(task *executor.Context, reader io.Reader, maxLineSize int, wait *sync.WaitGroup) {
	defer wait.Done()
	buffered := bufio.NewReader(reader)
	line := make([]byte, 0, min(maxLineSize, 4096))
	truncated := false
	for {
		// ReadSlice 允许在不无限扩容的情况下逐段消费超长日志行。
		fragment, err := buffered.ReadSlice('\n')
		content := fragment
		if !errors.Is(err, bufio.ErrBufferFull) {
			content = bytes.TrimSuffix(content, []byte{'\n'})
			content = bytes.TrimSuffix(content, []byte{'\r'})
		}
		remaining := maxLineSize - len(line)
		if remaining > 0 {
			line = append(line, content[:min(len(content), remaining)]...)
		}
		if len(content) > remaining {
			truncated = true
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if len(line) > 0 {
			task.Log("%s", line)
		}
		if truncated {
			task.Log("[日志行超过 %d 字节，剩余内容已截断]", maxLineSize)
		}
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			task.SystemLogger().Error("读取脚本输出失败", elog.FieldErr(err))
			return
		}
		line = line[:0]
		truncated = false
	}
}

func streamResult(task *executor.Context, reader io.Reader, maximum int64, wait *sync.WaitGroup) {
	defer wait.Done()
	decoder := json.NewDecoder(reader)
	exceeded := false
	for {
		// 结果通道允许连续写入多个 JSON 对象，Context 会逐个合并。
		var partial map[string]any
		err := decoder.Decode(&partial)
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			task.SystemLogger().Error("解析流式结果碎片失败", elog.FieldErr(err))
			_, _ = io.Copy(io.Discard, reader)
			return
		}
		if decoder.InputOffset() <= maximum {
			task.AddResult(partial)
		} else if !exceeded {
			exceeded = true
			task.SystemLogger().Error("脚本结果超过大小限制", elog.Int64("maxResultSize", maximum))
		}
	}
}

// MergeEnvironment 使用 overrides 覆盖 base 中的同名环境变量。
func MergeEnvironment(base, overrides []string) []string {
	overrideKeys := lo.SliceToMap(overrides, func(item string) (string, struct{}) {
		key, _, _ := strings.Cut(item, "=")
		return key, struct{}{}
	})
	result := lo.Filter(base, func(item string, _ int) bool {
		key, _, _ := strings.Cut(item, "=")
		_, exists := overrideKeys[key]
		return !exists
	})
	return append(result, overrides...)
}
