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

const maxHTTPResponseBytes = 4 << 20
const maxHTTPErrorBodyBytes = 4 << 10

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
	if exec.Task.HTTPConfig == nil {
		return domain.ExecutionState{}, fmt.Errorf("HTTP执行配置不能为空")
	}
	if exec.Task.HTTPConfig.Endpoint == "" {
		return domain.ExecutionState{}, fmt.Errorf("HTTP执行地址不能为空")
	}

	// 构造请求参数 - 使用实际的任务参数而非硬编码数据
	requestData := map[string]any{
		"taskId":      exec.Task.ID,
		"taskName":    exec.Task.Name,
		"executionId": exec.ID,
		"params":      httpParams(exec),
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

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxHTTPResponseBytes+1))
	if err != nil {
		return domain.ExecutionState{}, fmt.Errorf("读取HTTP响应失败: %w", err)
	}
	if len(body) > maxHTTPResponseBytes {
		return domain.ExecutionState{}, fmt.Errorf("HTTP响应超过大小限制: %d bytes", maxHTTPResponseBytes)
	}

	// 状态码判断
	if resp.StatusCode >= 400 {
		errorBody := body[:min(len(body), maxHTTPErrorBodyBytes)]
		return domain.ExecutionState{}, fmt.Errorf(
			"HTTP请求失败 status=%d body=%q",
			resp.StatusCode,
			string(errorBody),
		)
	}

	i.logger.Info("收到HTTP执行节点响应",
		elog.Int("statusCode", resp.StatusCode),
		elog.Int("responseBytes", len(body)))

	return domain.ExecutionState{
		ID:              exec.ID,
		TaskID:          exec.Task.ID,
		TaskName:        exec.Task.Name,
		Status:          domain.TaskExecutionStatusSuccess,
		RunningProgress: 100,
		TaskResult:      string(body),
	}, nil
}

func httpParams(exec domain.TaskExecution) map[string]string {
	params := make(map[string]string, len(exec.Task.HTTPConfig.Params)+len(exec.Task.ScheduleParams))
	for key, value := range exec.Task.HTTPConfig.Params {
		params[key] = value
	}
	for key, value := range exec.Task.ScheduleParams {
		params[key] = value
	}
	return params
}

func (i *HTTPInvoker) Terminate(context.Context, domain.TaskExecution, string) error { return nil }
