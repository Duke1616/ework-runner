package submission

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Duke1616/etask/internal/repository"
	"github.com/stretchr/testify/require"
)

func TestValidateCommand(t *testing.T) {
	testCases := []struct {
		name    string
		command RunRunnerCommand
		wantErr string
	}{
		{name: "合法请求", command: RunRunnerCommand{RequestID: "eflow:1:1", RunnerID: 10,
			Params: map[string]string{"args": `{"ticket_id":1}`}}},
		{name: "缺少幂等标识", command: RunRunnerCommand{RunnerID: 10}, wantErr: "幂等请求标识不能为空"},
		{name: "执行单元非法", command: RunRunnerCommand{RequestID: "eflow:1:1"}, wantErr: "执行单元 ID 非法"},
		{name: "参数不是 JSON", command: RunRunnerCommand{RequestID: "eflow:1:1", RunnerID: 10,
			Params: map[string]string{"args": "{"}}, wantErr: "必须是合法 JSON"},
		{name: "空参数使用默认值", command: RunRunnerCommand{RequestID: "eflow:1:1", RunnerID: 10,
			Params: map[string]string{"args": "  "}}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := validateCommand(testCase.command)
			if testCase.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, testCase.wantErr)
		})
	}
}

func TestMissingExecutionPoolIsRejected(t *testing.T) {
	err := fmt.Errorf("查询执行资源池失败: %w", repository.ErrExecutionPoolNotFound)

	require.True(t, errors.Is(classifyRouteError("sd_cdc_docker_net_env_runner", err), ErrRejected))
}
