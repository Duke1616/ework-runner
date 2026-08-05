package grpc

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/Duke1616/etask/pkg/grpc/registry"
	"github.com/gotomicro/ego/core/elog"
	"github.com/stretchr/testify/require"
	grpcresolver "google.golang.org/grpc/resolver"
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

func TestExecutorResolverClearsAddressesWhenRegistryIsEmpty(t *testing.T) {
	cc := &resolverClientConnStub{}
	r := &executorResolver{
		target: grpcresolver.Target{URL: url.URL{Path: "/executor"}},
		cc:     cc, registry: resolverRegistryStub{}, ctx: t.Context(), timeout: time.Second,
		logger: elog.DefaultLogger.With(elog.FieldComponentName("resolver.test")),
	}

	r.reconcile()

	require.Len(t, cc.states, 1)
	require.Empty(t, cc.states[0].Addresses)
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
	events    <-chan registry.Event
	instances []registry.ServiceInstance
}

func (r resolverRegistryStub) Register(context.Context, registry.ServiceInstance) error { return nil }
func (r resolverRegistryStub) UnRegister(context.Context, registry.ServiceInstance) error {
	return nil
}
func (r resolverRegistryStub) ListServices(context.Context, string) ([]registry.ServiceInstance, error) {
	return r.instances, nil
}

type resolverClientConnStub struct {
	grpcresolver.ClientConn
	states []grpcresolver.State
}

func (c *resolverClientConnStub) UpdateState(state grpcresolver.State) error {
	c.states = append(c.states, state)
	return nil
}

func (c *resolverClientConnStub) ReportError(error)                   {}
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
