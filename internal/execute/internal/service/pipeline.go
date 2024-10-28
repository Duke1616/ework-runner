package service

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

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
