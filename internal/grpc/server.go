package grpc

import (
	"context"
	"fmt"
	"net"

	"github.com/Duke1616/ecmdb/pkg/grpc/registry"
	"github.com/gotomicro/ego/core/constant"
	"github.com/gotomicro/ego/core/elog"
	"github.com/gotomicro/ego/server"
	"google.golang.org/grpc"
)

type Server struct {
	*grpc.Server
	id       string
	name     string
	reg      registry.Registry
	logger   *elog.Component
	addr     string
	listener net.Listener
	ctx      context.Context
	cancel   context.CancelFunc
}

func NewServer(
	id string,
	name string,
	addr string,
	reg registry.Registry,
) *Server {
	grpcServer := grpc.NewServer()
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		Server: grpcServer,
		name:   name,
		reg:    reg,
		addr:   addr,
		logger: elog.DefaultLogger,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Name 实现 server.Server 接口
func (s *Server) Name() string {
	return fmt.Sprintf("gRPC-%s", s.name)
}

// PackageName 实现 server.Server 接口
func (s *Server) PackageName() string {
	return "grpc.Server"
}

// Init 实现 server.Server 接口
func (s *Server) Init() error {
	return nil
}

// Start 实现 server.Server 接口
func (s *Server) Start() error {
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.listener = listener

	// 注册服务到 etcd
	err = s.reg.Register(context.Background(), registry.ServiceInstance{
		ID:      s.id,
		Name:    s.name,
		Address: listener.Addr().String(),
	})
	if err != nil {
		listener.Close()
		return err
	}

	s.logger.Info("gRPC 服务器启动", elog.String("地址", listener.Addr().String()))

	// NOTE: 在独立 goroutine 中启动 gRPC server，避免阻塞
	go func() {
		if err = s.Server.Serve(listener); err != nil {
			s.logger.Error("gRPC 服务器错误", elog.FieldErr(err))
		}
	}()

	return nil
}

// Stop 实现 server.Server 接口
func (s *Server) Stop() error {
	s.logger.Info("停止 gRPC 服务器")
	s.cancel()
	s.Server.Stop()
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

// GracefulStop 实现 server.Server 接口
func (s *Server) GracefulStop(_ context.Context) error {
	s.logger.Info("优雅停止 gRPC 服务器")
	s.cancel()
	s.Server.GracefulStop()
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

// Info 实现 server.Server 接口
func (s *Server) Info() *server.ServiceInfo {
	info := server.ApplyOptions(
		server.WithName(s.Name()),
		server.WithKind(constant.ServiceProvider),
	)
	info.Healthy = s.ctx.Err() == nil
	return &info
}
