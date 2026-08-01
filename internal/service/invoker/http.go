package invoker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/gotomicro/ego/core/elog"
)

var _ Invoker = &HTTPInvoker{}

type HTTPInvoker struct {
	logger *elog.Component
	client *http.Client
}

func NewHTTPInvoker() *HTTPInvoker {
	// 创建HTTP客户端，设置合理的超时时间
	const timeout = 30 * time.Second
	return &HTTPInvoker{
		logger: elog.DefaultLogger,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (i *HTTPInvoker) Name() string {
	return "HTTP"
}

func (i *HTTPInvoker) Run(ctx context.Context, exec domain.TaskExecution) (domain.ExecutionState, error) {
	// TODO: 实现HTTP客户端调用，当前为占位实现
	i.logger.Warn("HTTP执行方式尚未完全实现，使用占位逻辑",
		elog.Int64("taskId", exec.Task.ID),
		elog.String("endpoint", exec.Task.HTTPConfig.Endpoint))

	// 构造请求参数 - 使用实际的任务参数而非硬编码数据
	requestData := map[string]any{
		"taskId":      exec.Task.ID,
		"taskName":    exec.Task.Name,
		"executionId": exec.ID,
		"params":      exec.Task.HTTPConfig.Params,
	}
	// 将参数转换为JSON
	jsonBytes, err := json.Marshal(requestData)
	if err != nil {
		return domain.ExecutionState{}, fmt.Errorf("序列化请求参数失败: %w", err)
	}

	// 创建带有context的HTTP请求
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		exec.Task.HTTPConfig.Endpoint,
		bytes.NewReader(jsonBytes),
	)
	if err != nil {
		return domain.ExecutionState{}, fmt.Errorf("创建HTTP请求失败: %w", err)
	}

	// 默认 header
	req.Header.Set("Content-Type", "application/json")

	// 自定义 header（覆盖默认）
	for k, v := range exec.Task.HTTPConfig.Headers {
		req.Header.Set(k, v)
	}

	resp, err := i.client.Do(req)
	if err != nil {
		return domain.ExecutionState{}, fmt.Errorf("发送HTTP请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.ExecutionState{}, fmt.Errorf("读取HTTP响应失败: %w", err)
	}

	// 状态码判断
	if resp.StatusCode >= 400 {
		return domain.ExecutionState{}, fmt.Errorf(
			"HTTP请求失败 status=%d body=%s",
			resp.StatusCode,
			string(body),
		)
	}

	i.logger.Info("收到HTTP执行节点响应",
		elog.String("response", string(body)),
		elog.Int("statusCode", resp.StatusCode))

	return domain.ExecutionState{}, nil
}

func (i *HTTPInvoker) Terminate(context.Context, domain.TaskExecution, string) error { return nil }
