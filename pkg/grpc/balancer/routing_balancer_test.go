package balancer

import (
	"testing"

	grpcbalancer "google.golang.org/grpc/balancer"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/resolver"
)

type routingTestSubConn struct {
	grpcbalancer.SubConn
	shutdown bool
}

func (s *routingTestSubConn) Connect()  {}
func (s *routingTestSubConn) Shutdown() { s.shutdown = true }

type routingTestClientConn struct {
	grpcbalancer.ClientConn
	subConn *routingTestSubConn
	states  []grpcbalancer.State
}

func (c *routingTestClientConn) NewSubConn([]resolver.Address,
	grpcbalancer.NewSubConnOptions) (grpcbalancer.SubConn, error) {
	c.subConn = &routingTestSubConn{}
	return c.subConn, nil
}

func (c *routingTestClientConn) UpdateState(state grpcbalancer.State) {
	c.states = append(c.states, state)
}

func TestRoutingBalancerRemovesReadySubConnWithAddress(t *testing.T) {
	cc := &routingTestClientConn{}
	b := newRoutingBalancer(cc)
	if err := b.UpdateClientConnState(grpcbalancer.ClientConnState{ResolverState: resolver.State{
		Addresses: []resolver.Address{{Addr: "127.0.0.1:8080"}},
	}}); err != nil {
		t.Fatal(err)
	}
	b.UpdateSubConnState(cc.subConn, grpcbalancer.SubConnState{ConnectivityState: connectivity.Ready})
	if len(b.readySCs) != 1 {
		t.Fatalf("ready SubConns = %d, want 1", len(b.readySCs))
	}

	if err := b.UpdateClientConnState(grpcbalancer.ClientConnState{ResolverState: resolver.State{}}); err != nil {
		t.Fatal(err)
	}
	if !cc.subConn.shutdown {
		t.Fatal("removed SubConn was not shut down")
	}
	if len(b.readySCs) != 0 {
		t.Fatalf("ready SubConns after removal = %d, want 0", len(b.readySCs))
	}
	last := cc.states[len(cc.states)-1]
	if last.ConnectivityState != connectivity.Connecting {
		t.Fatalf("connectivity state = %s, want CONNECTING", last.ConnectivityState)
	}
}
