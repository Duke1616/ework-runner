package sse

import (
	"context"
	"encoding/json"
	"time"

	ssekit "github.com/Duke1616/etask/pkg/sse"
	"github.com/ecodeclub/ginx"
	"github.com/gotomicro/ego/core/elog"
	"github.com/redis/go-redis/v9"
)

const publishQueueSize = 1024

type redisEnvelope[K comparable, T any] struct {
	Source string `json:"source"`
	Key    K      `json:"key"`
	Event  T      `json:"event"`
}

// RedisTopicHub 将当前进程的 SSE TopicHub 与 Redis Pub/Sub 连接起来。
type RedisTopicHub[K comparable, T any] struct {
	local    *ssekit.TopicHub[K, T]
	client   redis.UniversalClient
	channel  string
	instance string
	logger   *elog.Component
	outbound chan []byte
}

// NewRedisTopicHub 创建支持跨 Scheduler 实例广播的 TopicHub。
func NewRedisTopicHub[K comparable, T any](
	client redis.UniversalClient,
	channel string,
	instance string,
) *RedisTopicHub[K, T] {
	return &RedisTopicHub[K, T]{
		local:    ssekit.NewTopicHub[K, T](),
		client:   client,
		channel:  channel,
		instance: instance,
		logger: elog.DefaultLogger.With(
			elog.FieldComponentName("sse.RedisTopicHub"),
			elog.String("channel", channel),
		),
		outbound: make(chan []byte, publishQueueSize),
	}
}

// Broadcast 立即通知当前进程，并发布到 Redis 供其他实例转发。
func (h *RedisTopicHub[K, T]) Broadcast(key K, evt T) {
	h.local.BroadcastLocal(key, evt)
	if h.client == nil {
		return
	}
	payload, err := json.Marshal(redisEnvelope[K, T]{
		Source: h.instance,
		Key:    key,
		Event:  evt,
	})
	if err != nil {
		h.logger.Error("序列化 SSE 广播事件失败", elog.FieldErr(err))
		return
	}
	select {
	case h.outbound <- payload:
	default:
		h.logger.Warn("SSE Redis 发布队列已满，丢弃跨实例事件")
	}
}

// Subscribe 订阅当前实例指定业务键的事件。
func (h *RedisTopicHub[K, T]) Subscribe(key K) chan T {
	return h.local.Subscribe(key)
}

// Unsubscribe 取消当前实例指定业务键的订阅。
func (h *RedisTopicHub[K, T]) Unsubscribe(key K, ch chan T) {
	h.local.Unsubscribe(key, ch)
}

// Stream 将指定业务键的本地事件流写入当前 HTTP SSE 连接。
func (h *RedisTopicHub[K, T]) Stream(
	ctx *ginx.Context,
	key K,
	eventName string,
	heartbeat time.Duration,
) {
	h.local.Stream(ctx, key, eventName, heartbeat)
}

// Start 订阅 Redis 广播，并把其他实例发布的事件写入当前进程的本地 Hub。
func (h *RedisTopicHub[K, T]) Start(ctx context.Context) {
	if h.client == nil {
		return
	}
	go h.publishLoop(ctx)
	h.subscribeLoop(ctx)
}

func (h *RedisTopicHub[K, T]) publishLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case payload := <-h.outbound:
			if err := h.client.Publish(ctx, h.channel, payload).Err(); err != nil && ctx.Err() == nil {
				h.logger.Error("发布 SSE 广播事件失败", elog.FieldErr(err))
			}
		}
	}
}

func (h *RedisTopicHub[K, T]) subscribeLoop(ctx context.Context) {
	for ctx.Err() == nil {
		sub := h.client.Subscribe(ctx, h.channel)
		if _, err := sub.Receive(ctx); err != nil {
			_ = sub.Close()
			if ctx.Err() != nil {
				return
			}
			h.logger.Error("建立 SSE Redis 订阅失败", elog.FieldErr(err))
			if !waitRetry(ctx) {
				return
			}
			continue
		}

		messages := sub.Channel()
		connected := true
		for connected {
			select {
			case <-ctx.Done():
				_ = sub.Close()
				return
			case message, ok := <-messages:
				if !ok {
					connected = false
					continue
				}
				h.consume([]byte(message.Payload))
			}
		}
		_ = sub.Close()
		if !waitRetry(ctx) {
			return
		}
	}
}

func (h *RedisTopicHub[K, T]) consume(payload []byte) {
	var envelope redisEnvelope[K, T]
	if err := json.Unmarshal(payload, &envelope); err != nil {
		h.logger.Error("解析 SSE Redis 事件失败", elog.FieldErr(err))
		return
	}
	if envelope.Source == h.instance {
		return
	}
	h.local.BroadcastLocal(envelope.Key, envelope.Event)
}

func waitRetry(ctx context.Context) bool {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
