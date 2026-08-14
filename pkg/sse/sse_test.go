package sse

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ecodeclub/ginx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSSEFlushesHandshakeImmediately(t *testing.T) {
	gin.SetMode(gin.TestMode)
	events := make(chan string)
	router := gin.New()
	router.GET("/stream", func(ctx *gin.Context) {
		SSE(&ginx.Context{Context: ctx}, Subscription[string]{EventChan: events}, "test_event", time.Hour)
	})

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	client := &http.Client{Timeout: time.Second}
	resp, err := client.Get(server.URL + "/stream")
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
	require.Equal(t, "no", resp.Header.Get("X-Accel-Buffering"))

	line, err := bufio.NewReader(resp.Body).ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, ": connected\n", line)
}
