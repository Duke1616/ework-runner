package sse

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRedisTopicHubBroadcastsLocallyWithoutRedis(t *testing.T) {
	hub := NewRedisTopicHub[int64, TaskStatusEvent](nil, "etask:sse:tasks", "node-a")
	ch := hub.Subscribe(2)
	defer hub.Unsubscribe(2, ch)

	want := TaskStatusEvent{TaskID: 10, Status: "ACTIVE", Version: 3}
	hub.Broadcast(2, want)

	select {
	case got := <-ch:
		require.Equal(t, want, got)
	case <-time.After(time.Second):
		t.Fatal("未收到本地 SSE 事件")
	}
}

func TestRedisTopicHubForwardsRemoteEventAndIgnoresLoopback(t *testing.T) {
	hub := NewRedisTopicHub[int64, TaskStatusEvent](nil, "etask:sse:tasks", "node-a")
	ch := hub.Subscribe(2)
	defer hub.Unsubscribe(2, ch)

	remote := []byte(`{"source":"node-b","key":2,"event":{"task_id":10,"status":"PREEMPTED","version":4}}`)
	hub.consume(remote)
	require.Equal(t, int64(4), (<-ch).Version)

	loopback := []byte(`{"source":"node-a","key":2,"event":{"task_id":10,"status":"ACTIVE","version":5}}`)
	hub.consume(loopback)
	select {
	case event := <-ch:
		t.Fatalf("不应转发当前实例的 Redis 回环事件: %#v", event)
	default:
	}
}
