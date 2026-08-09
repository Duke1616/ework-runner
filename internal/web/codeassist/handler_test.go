package codeassist

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	codeassistSvc "github.com/Duke1616/etask/internal/service/codeassist"
	"github.com/ecodeclub/ginx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type serviceStub struct{ codeassistSvc.Service }

type panicServiceStub struct{ codeassistSvc.Service }

type delayedServiceStub struct{ codeassistSvc.Service }

func (panicServiceStub) Chat(context.Context, domain.AIChatRequest,
	codeassistSvc.EventEmitter) error {
	panic("provider panic")
}

func (delayedServiceStub) Chat(_ context.Context, _ domain.AIChatRequest,
	emit codeassistSvc.EventEmitter) error {
	if err := emit(codeassistSvc.StreamEvent{
		Type: codeassistSvc.StreamEventTypeStarted, MessageID: 10,
	}); err != nil {
		return err
	}
	time.Sleep(80 * time.Millisecond)
	if err := emit(codeassistSvc.StreamEvent{
		Type: codeassistSvc.StreamEventTypeDelta, MessageID: 10, Text: "第一段",
	}); err != nil {
		return err
	}
	time.Sleep(80 * time.Millisecond)
	if err := emit(codeassistSvc.StreamEvent{
		Type: codeassistSvc.StreamEventTypeDelta, MessageID: 10, Text: "第二段",
	}); err != nil {
		return err
	}
	return emit(codeassistSvc.StreamEvent{
		Type: codeassistSvc.StreamEventTypeCompleted, MessageID: 10,
	})
}

func (serviceStub) Chat(_ context.Context, _ domain.AIChatRequest,
	emit codeassistSvc.EventEmitter) error {
	if err := emit(codeassistSvc.StreamEvent{
		Type: codeassistSvc.StreamEventTypeStarted, MessageID: 10,
	}); err != nil {
		return err
	}
	if err := emit(codeassistSvc.StreamEvent{
		Type: codeassistSvc.StreamEventTypeDelta, MessageID: 10, Text: "正在分析",
	}); err != nil {
		return err
	}
	return emit(codeassistSvc.StreamEvent{
		Type: codeassistSvc.StreamEventTypeCompleted, MessageID: 10,
	})
}

func TestStreamChatWritesSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(response)
	ginContext.Request = httptest.NewRequest("POST", "/api/code-assist/message/stream", nil)
	handler := NewHandler(serviceStub{})

	_, err := handler.StreamChat(&ginx.Context{Context: ginContext}, ChatReq{
		ConversationID: 1, Content: "分析当前脚本",
	})

	require.ErrorIs(t, err, ginx.ErrNoResponse)
	require.Contains(t, response.Header().Get("Content-Type"), "text/event-stream")
	require.Contains(t, response.Body.String(), "event:message.started")
	require.Contains(t, response.Body.String(), "event:message.delta")
	require.Contains(t, response.Body.String(), "正在分析")
	require.Contains(t, response.Body.String(), "event:message.completed")
}

func TestStreamChatFlushesEventsImmediately(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(delayedServiceStub{})
	router.POST("/stream", ginx.B[ChatReq](handler.StreamChat))
	server := httptest.NewServer(router)
	defer server.Close()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/stream",
		strings.NewReader(`{"conversation_id":1,"content":"分析"}`))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	response, err := server.Client().Do(request)
	require.NoError(t, err)
	defer response.Body.Close()

	scanner := bufio.NewScanner(response.Body)
	deltaTimes := make([]time.Time, 0, 2)
	for scanner.Scan() {
		if scanner.Text() == "event:message.delta" {
			deltaTimes = append(deltaTimes, time.Now())
			if len(deltaTimes) == 2 {
				break
			}
		}
	}
	require.NoError(t, scanner.Err())
	require.Len(t, deltaTimes, 2)
	require.GreaterOrEqual(t, deltaTimes[1].Sub(deltaTimes[0]), 60*time.Millisecond)
}

func TestStreamChatRecoversBackgroundPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(response)
	ginContext.Request = httptest.NewRequest("POST", "/api/code-assist/message/stream", nil)
	handler := NewHandler(panicServiceStub{})

	_, err := handler.StreamChat(&ginx.Context{Context: ginContext}, ChatReq{
		ConversationID: 1, Content: "分析当前脚本",
	})

	require.ErrorIs(t, err, ginx.ErrNoResponse)
	require.Contains(t, response.Body.String(), "event:message.failed")
	require.Contains(t, response.Body.String(), "AI 请求失败")
	require.NotContains(t, response.Body.String(), "provider panic")
}

func TestTranslateError(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		code int
	}{
		{name: "参数错误", err: errs.ErrInvalidParameter, code: InvalidParameterCode},
		{name: "会话占用", err: errs.ErrAIConversationBusy, code: ConflictCode},
		{name: "变更集冲突", err: errs.ErrAIChangeSetConflict, code: ConflictCode},
		{name: "内部错误", err: context.DeadlineExceeded, code: SystemErrorCode},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			require.Equal(t, testCase.code, translateError(testCase.err).Code)
		})
	}
}
