package grpc

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/Duke1616/ework-runner/pkg/grpc/registry"
	"github.com/Duke1616/ework-runner/pkg/netx"
	"github.com/gotomicro/ego/core/constant"
	"github.com/gotomicro/ego/core/elog"
	"github.com/gotomicro/ego/server"
	"google.golang.org/grpc"
)

const (
	// ComponentName 日志组件名
	ComponentName = "grpc.server"
)

type Server struct {
	*grpc.Server
	// 服务注册相关
	registry       registry.Registry
	serviceID      string // 服务实例ID
	ServiceName    string
	listenAddr     string // 监听地址
	advertiseAddr  string // 广播地址(可选)
	registeredAddr string // 注册到注册中心的地址
	cancel         func()
	logger         *elog.Component
}

// NewServer 创建 gRPC Server 实例
func NewServer(id, name, listenAddr, advertiseAddr string, reg registry.Registry) *Server {
	return &Server{
		Server:        grpc.NewServer(),
		serviceID:     id,
		ServiceName:   name,
		registry:      reg,
		listenAddr:    listenAddr,
		advertiseAddr: advertiseAddr,
		logger:        elog.DefaultLogger.With(elog.FieldComponentName(ComponentName)),
	}
}

// resolveAdvertiseAddress 解析服务注册地址
func (s *Server) resolveAdvertiseAddress() (string, error) {
	// 1. 优先使用配置的 advertise_addr
	if s.advertiseAddr != "" {
		s.logger.Info("使用配置的广播地址",
			elog.String("advertiseAddr", s.advertiseAddr))
		return s.advertiseAddr, nil
	}

	// 2. 从 listenAddr 解析
	host, port, err := net.SplitHostPort(s.listenAddr)
	if err != nil {
		return "", fmt.Errorf("解析监听地址失败: %w", err)
	}

	// 3. 如果是通配符地址,智能解析 IP
	if host == "::" || host == "0.0.0.0" {
		ip, err := s.getAdvertiseIP()
		if err != nil {
			return "", fmt.Errorf("获取广播 IP 失败: %w", err)
		}
		return net.JoinHostPort(ip, port), nil
	}

	return s.listenAddr, nil
}

// getAdvertiseIP 智能获取广播 IP
func (s *Server) getAdvertiseIP() (string, error) {
	// 1. K8s 环境:优先使用 POD_IP
	if podIP := os.Getenv("POD_IP"); podIP != "" {
		s.logger.Info("使用 K8s Pod IP", elog.String("podIP", podIP))
		return podIP, nil
	}

	// 2. Docker 环境:使用 HOST_IP 环境变量
	if hostIP := os.Getenv("HOST_IP"); hostIP != "" {
		s.logger.Info("使用环境变量 HOST_IP", elog.String("hostIP", hostIP))
		return hostIP, nil
	}

	// 3. 裸机环境:自动检测本机 IP
	ip, err := netx.GetOutboundIP()
	if err != nil {
		return "", err
	}
	s.logger.Info("自动检测本机 IP", elog.String("ip", ip))
	return ip, nil
}

// startServer 启动服务器并注册到 etcd (内部方法)
func (s *Server) startServer() (net.Listener, error) {
	// 初始化控制上下文
	_, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	listener, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return nil, fmt.Errorf("监听端口失败: %w", err)
	}

	// 解析要注册的地址
	addr, err := s.resolveAdvertiseAddress()
	if err != nil {
		listener.Close()
		return nil, err
	}

	// 注册服务到 etcd
	if err = s.register(addr); err != nil {
		listener.Close() // 清理资源
		return nil, fmt.Errorf("注册服务失败: %w", err)
	}

	return listener, nil
}

// Serve 启动服务器并且阻塞
func (s *Server) Serve() error {
	listener, err := s.startServer()
	if err != nil {
		return err
	}
	return s.Server.Serve(listener)
}

func (s *Server) register(addr string) error {
	s.registeredAddr = addr
	s.logger.Info("注册服务到 etcd",
		elog.String("serviceID", s.serviceID),
		elog.String("serviceName", s.ServiceName),
		elog.String("addr", addr))

	// NOTE: 使用 registry.Registry 接口注册服务,租约管理由 Registry 内部处理
	return s.registry.Register(context.Background(), registry.ServiceInstance{
		ID:      s.serviceID,
		Name:    s.ServiceName,
		Address: addr,
	})
}

func (s *Server) Close() error {
	// 取消续约
	if s.cancel != nil {
		s.cancel()
	}

	// 注销服务
	if s.registry != nil {
		if err := s.registry.UnRegister(context.Background(), registry.ServiceInstance{
			ID:      s.serviceID,
			Name:    s.ServiceName,
			Address: s.registeredAddr,
		}); err != nil {
			s.logger.Error("注销服务失败", elog.FieldErr(err))
		}
	}

	s.Server.GracefulStop()
	return nil
}

// 以下方法实现 server.Server 接口，使其能被 ego 框架的 egoApp.Serve() 使用

// Name 实现 server.Server 接口
func (s *Server) Name() string {
	return s.ServiceName
}

// Init 实现 server.Server 接口
func (s *Server) Init() error {
	return nil
}

// Start 实现 server.Server 接口
func (s *Server) Start() error {
	listener, err := s.startServer()
	if err != nil {
		return err
	}

	// 异步启动 gRPC 服务
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
	return s.Close()
}

// GracefulStop 实现 server.Server 接口
func (s *Server) GracefulStop(ctx context.Context) error {
	s.logger.Info("优雅停止 gRPC 服务器")

	// 注销服务
	if s.registry != nil {
		if err := s.registry.UnRegister(context.Background(), registry.ServiceInstance{
			ID:      s.serviceID,
			Name:    s.ServiceName,
			Address: s.registeredAddr,
		}); err != nil {
			s.logger.Error("注销服务失败", elog.FieldErr(err))
		}
	}

	// 取消续约
	if s.cancel != nil {
		s.cancel()
	}

	// 优雅停止 gRPC Server
	s.Server.GracefulStop()

	return nil
}

// PackageName 实现 server.Server 接口
func (s *Server) PackageName() string {
	return ComponentName
}

// Info 实现 server.Server 接口
func (s *Server) Info() *server.ServiceInfo {
	info := server.ApplyOptions(
		server.WithName(s.ServiceName),
		server.WithKind(constant.ServiceProvider),
		server.WithScheme("grpc"),
		server.WithAddress(s.listenAddr),
	)

	// 判断服务是否健康
	info.Healthy = s.cancel != nil
	return &info
}
