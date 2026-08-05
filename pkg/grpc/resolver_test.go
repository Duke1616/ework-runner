package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/Duke1616/etask/pkg/grpc/registry"
	"github.com/stretchr/testify/require"
)

func TestExecutorResolverWatchStopsWhenSubscriptionCloses(t *testing.T) {
	events := make(chan registry.Event)
	close(events)
	r := &executorResolver{
		registry:     resolverRegistryStub{events: events},
		close:        make(chan struct{}),
		updateNotify: make(chan struct{}, 1),
	}

	done := make(chan struct{})
	go func() {
		r.watch()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watch 没有在订阅关闭后退出")
	}
	require.Empty(t, r.updateNotify)
}

func TestExecutorResolverCloseCancelsContextSubscription(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	registryStub := &contextResolverRegistryStub{
		resolverRegistryStub: resolverRegistryStub{events: make(chan registry.Event)},
		contexts:             make(chan context.Context, 1),
	}
	r := &executorResolver{
		registry: registryStub, close: make(chan struct{}),
		ctx: ctx, cancel: cancel, updateNotify: make(chan struct{}, 1),
	}
	done := make(chan struct{})
	go func() {
		r.watch()
		close(done)
	}()

	var subscriptionCtx context.Context
	select {
	case subscriptionCtx = <-registryStub.contexts:
	case <-time.After(time.Second):
		t.Fatal("resolver 没有创建 context 订阅")
	}
	r.Close()
	r.Close()

	select {
	case <-subscriptionCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("Close 没有取消 Registry 订阅")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watch 没有在 resolver 关闭后退出")
	}
}

type resolverRegistryStub struct {
	events <-chan registry.Event
}

func (r resolverRegistryStub) Register(context.Context, registry.ServiceInstance) error { return nil }
func (r resolverRegistryStub) UnRegister(context.Context, registry.ServiceInstance) error {
	return nil
}
func (r resolverRegistryStub) ListServices(context.Context, string) ([]registry.ServiceInstance, error) {
	return nil, nil
}
func (r resolverRegistryStub) Subscribe(string) <-chan registry.Event { return r.events }
func (resolverRegistryStub) Close() error                             { return nil }

type contextResolverRegistryStub struct {
	resolverRegistryStub
	contexts chan context.Context
}

func (r *contextResolverRegistryStub) SubscribeContext(ctx context.Context, _ string) <-chan registry.Event {
	r.contexts <- ctx
	return r.events
}
