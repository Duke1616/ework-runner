package pool

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Duke1616/etask/pkg/grpc/registry"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

func TestClientsCreatesOneConnectionPerService(t *testing.T) {
	const callers = 8
	var created atomic.Int32
	clients := NewClients[*grpc.ClientConn](poolRegistryStub{}, time.Second, "", func(conn *grpc.ClientConn) *grpc.ClientConn {
		created.Add(1)
		return conn
	})

	results := make([]*grpc.ClientConn, callers)
	var calls sync.WaitGroup
	calls.Add(callers)
	for index := range callers {
		go func() {
			defer calls.Done()
			results[index] = clients.Get("executor")
		}()
	}
	calls.Wait()

	require.Equal(t, int32(1), created.Load())
	for _, result := range results {
		require.Same(t, results[0], result)
	}

	require.NoError(t, clients.Close())
	require.NoError(t, clients.Close())
	require.Equal(t, connectivity.Shutdown, results[0].GetState())
}

func TestClientsRejectsGetAfterClose(t *testing.T) {
	clients := NewClients[*grpc.ClientConn](poolRegistryStub{}, time.Second, "", func(conn *grpc.ClientConn) *grpc.ClientConn {
		return conn
	})
	require.NoError(t, clients.Close())
	require.PanicsWithValue(t, "grpc client pool is closed", func() { clients.Get("executor") })
	_, err := clients.ListServices(t.Context(), "executor")
	require.ErrorContains(t, err, "pool is closed")
}

type poolRegistryStub struct{}

func (poolRegistryStub) Register(context.Context, registry.ServiceInstance) error   { return nil }
func (poolRegistryStub) UnRegister(context.Context, registry.ServiceInstance) error { return nil }
func (poolRegistryStub) ListServices(context.Context, string) ([]registry.ServiceInstance, error) {
	return nil, nil
}
func (poolRegistryStub) Subscribe(string) <-chan registry.Event { return make(chan registry.Event) }
func (poolRegistryStub) Close() error                           { return nil }
