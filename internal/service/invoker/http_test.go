package invoker

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestHTTPInvokerRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "custom", r.Header.Get("X-Test"))
		var request map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		require.Equal(t, float64(10), request["executionId"])
		require.Equal(t, map[string]any{"input": "scheduled"}, request["params"])
		_, _ = w.Write([]byte(`{"result":"ok"}`))
	}))
	defer server.Close()

	execution := domain.TaskExecution{
		ID: 10,
		Task: domain.Task{
			ID: 20, Name: "HTTP task",
			HTTPConfig: &domain.HTTPConfig{
				Endpoint: server.URL,
				Headers:  map[string]string{"X-Test": "custom"},
				Params:   map[string]string{"input": "value"},
			},
			ScheduleParams: map[string]string{"input": "scheduled"},
		},
	}
	state, err := NewHTTPInvoker().Run(t.Context(), execution)
	require.NoError(t, err)
	require.Equal(t, execution.ID, state.ID)
	require.Equal(t, execution.Task.ID, state.TaskID)
	require.Equal(t, domain.TaskExecutionStatusSuccess, state.Status)
	require.Equal(t, int32(100), state.RunningProgress)
	require.JSONEq(t, `{"result":"ok"}`, state.TaskResult)
}

func TestHTTPInvokerRunRejectsInvalidResponse(t *testing.T) {
	t.Run("非成功状态码", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		}))
		defer server.Close()

		_, err := NewHTTPInvoker().Run(t.Context(), httpExecution(server.URL))
		require.ErrorContains(t, err, "status=503")
	})

	t.Run("响应过大", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(strings.Repeat("x", maxHTTPResponseBytes+1)))
		}))
		defer server.Close()

		_, err := NewHTTPInvoker().Run(t.Context(), httpExecution(server.URL))
		require.ErrorContains(t, err, "超过大小限制")
	})
}

func TestHTTPInvokerRunValidatesConfig(t *testing.T) {
	_, err := NewHTTPInvoker().Run(t.Context(), domain.TaskExecution{})
	require.ErrorContains(t, err, "配置不能为空")

	execution := httpExecution("")
	_, err = NewHTTPInvoker().Run(t.Context(), execution)
	require.ErrorContains(t, err, "地址不能为空")
}

func httpExecution(endpoint string) domain.TaskExecution {
	return domain.TaskExecution{
		ID:   10,
		Task: domain.Task{ID: 20, Name: "HTTP task", HTTPConfig: &domain.HTTPConfig{Endpoint: endpoint}},
	}
}
