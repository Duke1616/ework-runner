package invoker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Duke1616/ework-runner/internal/domain"
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, exec.Task.HTTPConfig.Endpoint, strings.NewReader(string(jsonBytes)))
	if err != nil {
		return domain.ExecutionState{}, fmt.Errorf("创建HTTP请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// 发送POST请求到执行节点
	resp, err := i.client.Do(req)
	if err != nil {
		return domain.ExecutionState{}, fmt.Errorf("发送HTTP请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.ExecutionState{}, fmt.Errorf("读取HTTP响应失败: %w", err)
	}

	i.logger.Info("收到HTTP执行节点响应",
		elog.String("response", string(body)),
		elog.Int("statusCode", resp.StatusCode))

	return domain.ExecutionState{}, nil
}

func (i *HTTPInvoker) Prepare(_ context.Context, _ domain.TaskExecution) (map[string]string, error) {
	// TODO implement me
	panic("implement me")
}
