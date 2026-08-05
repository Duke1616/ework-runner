package grpc

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestServerGracefulStopHonorsContext(t *testing.T) {
	server, address, handler := startBlockingServer(t)
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	callDone := make(chan error, 1)
	go func() {
		callDone <- conn.Invoke(context.Background(), "/test.Blocker/Block", &emptypb.Empty{}, &emptypb.Empty{})
	}()
	select {
	case <-handler.started:
	case <-time.After(time.Second):
		t.Fatal("RPC 没有开始执行")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	started := time.Now()
	err = server.GracefulStop(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(started), time.Second)

	close(handler.release)
	select {
	case <-callDone:
	case <-time.After(time.Second):
		t.Fatal("强制停止后 RPC 没有退出")
	}
}

func TestServerStopDoesNotWaitForActiveRPC(t *testing.T) {
	server, address, handler := startBlockingServer(t)
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	go func() {
		_ = conn.Invoke(context.Background(), "/test.Blocker/Block", &emptypb.Empty{}, &emptypb.Empty{})
	}()
	select {
	case <-handler.started:
	case <-time.After(time.Second):
		t.Fatal("RPC 没有开始执行")
	}

	started := time.Now()
	require.NoError(t, server.Stop())
	require.Less(t, time.Since(started), time.Second)
	close(handler.release)
}

func TestServerInitValidatesConfig(t *testing.T) {
	server := NewServer(ServerConfig{}, nil)
	require.ErrorContains(t, server.Init(), "ServiceName")
}

func startBlockingServer(t *testing.T) (*Server, string, *blockingService) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	grpcServer := grpc.NewServer()
	handler := &blockingService{started: make(chan struct{}), release: make(chan struct{})}
	grpcServer.RegisterService(&blockingServiceDesc, handler)
	server := NewServer(ServerConfig{}, nil)
	server.Server = grpcServer
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = listener.Close()
	})
	return server, listener.Addr().String(), handler
}

type blockingServiceServer interface {
	Block(context.Context, *emptypb.Empty) (*emptypb.Empty, error)
}

type blockingService struct {
	started chan struct{}
	release chan struct{}
}

func (s *blockingService) Block(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
	close(s.started)
	<-s.release
	return &emptypb.Empty{}, nil
}

var blockingServiceDesc = grpc.ServiceDesc{
	ServiceName: "test.Blocker",
	HandlerType: (*blockingServiceServer)(nil),
	Methods: []grpc.MethodDesc{{
		MethodName: "Block",
		Handler: func(srv any, ctx context.Context, decode func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
			request := &emptypb.Empty{}
			if err := decode(request); err != nil {
				return nil, err
			}
			return srv.(blockingServiceServer).Block(ctx, request)
		},
	}},
}
