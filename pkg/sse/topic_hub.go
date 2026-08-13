package sse

import (
	"sync"
	"time"

	"github.com/ecodeclub/ginx"
)

// TopicHub 支持按 Key 路由订阅与广播的线程安全 SSE 中心
type TopicHub[K comparable, T any] struct {
	mu      sync.RWMutex
	clients map[K]map[chan T]bool
}

// NewTopicHub 实例化一个新的全局多 Topic 事件 Hub
func NewTopicHub[K comparable, T any]() *TopicHub[K, T] {
	return &TopicHub[K, T]{
		clients: make(map[K]map[chan T]bool),
	}
}

// Subscribe 订阅指定 Key 的事件
func (h *TopicHub[K, T]) Subscribe(key K) chan T {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.clients[key]; !exists {
		h.clients[key] = make(map[chan T]bool)
	}

	ch := make(chan T, 10)
	h.clients[key][ch] = true
	return ch
}

// Unsubscribe 取消订阅并安全关闭通道
func (h *TopicHub[K, T]) Unsubscribe(key K, ch chan T) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if clients, exists := h.clients[key]; exists {
		if _, clientExists := clients[ch]; clientExists {
			delete(clients, ch)
			close(ch)
		}
		if len(clients) == 0 {
			delete(h.clients, key)
		}
	}
}

// Broadcast 向指定 Key 广播事件
func (h *TopicHub[K, T]) Broadcast(key K, evt T) {
	h.BroadcastLocal(key, evt)
}

// BroadcastLocal 仅向当前进程的订阅者发送事件，不触发外部发布器。
// 分布式 Hub 收到 Redis 转发事件时使用该方法，避免事件再次回到 Redis。
func (h *TopicHub[K, T]) BroadcastLocal(key K, evt T) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	clients, exists := h.clients[key]
	if !exists {
		return
	}

	for ch := range clients {
		select {
		case ch <- evt:
		default:
			// 慢速客户端/缓冲区满，跳过以避免阻塞
		}
	}
}

// Stream 启动特定 Key 路由下的 SSE 推送，自动处理生命周期与清理
func (h *TopicHub[K, T]) Stream(ctx *ginx.Context, key K, eventName string, heartbeat time.Duration) {
	ch := h.Subscribe(key)
	sub := Subscription[T]{
		EventChan: ch,
		CloseFunc: func() {
			h.Unsubscribe(key, ch)
		},
	}
	SSE(ctx, sub, eventName, heartbeat)
}
