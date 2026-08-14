package sse

import (
	"io"
	"time"

	"github.com/ecodeclub/ginx"
)

// Subscription SSE 订阅接口返回值，包含事件通道和一个清理资源的关闭函数
type Subscription[T any] struct {
	EventChan <-chan T
	CloseFunc func()
}

// SSE 执行通用的 SSE 流式推送逻辑
func SSE[T any](ctx *ginx.Context, sub Subscription[T], eventName string, heartbeat time.Duration) {
	ctx.Header("Content-Type", "text/event-stream")
	ctx.Header("Cache-Control", "no-cache, no-transform")

	// 强制禁用网关或反向代理的 Buffer 缓存，保证流式推送的实时性
	ctx.Header("X-Accel-Buffering", "no")
	if sub.CloseFunc != nil {
		defer sub.CloseFunc()
	}

	// 立即提交响应头和首个注释帧，避免在第一个业务事件或心跳到来前，
	// 浏览器一直停留在 Provisional headers 状态。
	if _, err := ctx.Writer.WriteString(": connected\n\n"); err != nil {
		return
	}
	ctx.Writer.Flush()

	hb := heartbeat
	if hb <= 0 {
		hb = 20 * time.Second
	}
	ticker := time.NewTicker(hb)
	defer ticker.Stop()

	ctx.Stream(func(w io.Writer) bool {
		select {
		case event, ok := <-sub.EventChan:
			if !ok {
				return false
			}
			ctx.SSEvent(eventName, event)
			return true
		case <-ticker.C:
			// 发送以冒号 ":" 开头的行作为心跳注释，客户端会忽略，用于维持连接活跃度并及时检测异常中断
			_, err := w.Write([]byte(": ping\n\n"))
			return err == nil
		case <-ctx.Request.Context().Done():
			// 客户端主动断开连接，安全释放流式通道协程
			return false
		}
	})
}

// Stream 启动针对该 Hub 的 SSE 事件流推送，自动处理订阅与安全关闭，减少 Handler 的样板代码
func (h *Hub[T]) Stream(ctx *ginx.Context, eventName string, heartbeat time.Duration) {
	ch := h.Subscribe()
	sub := Subscription[T]{
		EventChan: ch,
		CloseFunc: func() {
			h.Unsubscribe(ch)
		},
	}
	SSE(ctx, sub, eventName, heartbeat)
}
