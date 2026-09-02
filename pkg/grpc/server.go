package grpc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync/atomic"
	"time"

	"github.com/Duke1616/etask/pkg/grpc/interceptors"
	"github.com/Duke1616/etask/pkg/grpc/registry"
	"github.com/Duke1616/etask/pkg/netx"
	"github.com/gotomicro/ego/core/constant"
	"github.com/gotomicro/ego/core/elog"
	"github.com/gotomicro/ego/server"
	"google.golang.org/grpc"
)

const (
	// ComponentName 日志组件名
	ComponentName = "grpc.server"
)

type ServerConfig struct {
	ServiceId     string `mapstructure:"id"`             // 可选:实例ID，执行节点必填
	ServiceName   string `mapstructure:"name"`           // 必填:服务名
	ListenAddr    string `mapstructure:"listen_addr"`    // 必填:绑定地址
	AdvertiseAddr string `mapstructure:"advertise_addr"` // 可选:手动指定注册到etcd的地址
	AuthToken     string `mapstructure:"auth_token"`     // 可选:认证令牌，如果需要认证就传递
}

// Validate 验证配置
func (c *ServerConfig) Validate() error {
	if c.ServiceName == "" {
		return fmt.Errorf("ServiceName 不能为空")
	}
	if c.ListenAddr == "" {
		return fmt.Errorf("监听地址不能为空")
	}
	return nil
}

type Server struct {
	*grpc.Server

	// 配置文件
	config ServerConfig

	// 服务注册相关
	registry       registry.Registry
	serviceID      string // 服务实例ID
	ServiceName    string
	listenAddr     string // 监听地址
	advertiseAddr  string // 广播地址(可选)
	registeredAddr string // 注册到注册中心的地址
	started        atomic.Bool
	logger         *elog.Component
	metadata       map[string]any // 附加元数据
}

// ServerOption Server 配置选项
type ServerOption func(*Server)

// WithJWTAuth 启用 JWT 认证
// 如果 authToken 为空,则不启用认证
func WithJWTAuth(authToken string) ServerOption {
	return func(s *Server) {
		s.config.AuthToken = authToken
	}
}

// WithMetadata 设置服务注册元数据
func WithMetadata(metadata map[string]any) ServerOption {
	return func(s *Server) {
		s.metadata = metadata
	}
}

// NewServer 创建 gRPC Server 实例
func NewServer(cfg ServerConfig, reg registry.Registry, opts ...ServerOption) *Server {
	s := &Server{
		config:        cfg,
		registry:      reg,
		serviceID:     cfg.ServiceId,
		ServiceName:   cfg.ServiceName,
		listenAddr:    cfg.ListenAddr,
		advertiseAddr: cfg.AdvertiseAddr,
		logger:        elog.DefaultLogger.With(elog.FieldComponentName(ComponentName)),
	}

	// 1. 应用自定义配置选项
	for _, opt := range opts {
		opt(s)
	}

	// 2. 管道化构建一元及流式拦截器链
	unaryInterceptors, streamInterceptors := s.buildInterceptors()

	// 3. 最终创建包含所有拦截器链的 gRPC Server
	s.Server = grpc.NewServer(
		grpc.ChainUnaryInterceptor(unaryInterceptors...),
		grpc.ChainStreamInterceptor(streamInterceptors...),
	)

	return s
}

// buildInterceptors 管道化组装服务端拦截器链，通过统一的门面 (Facade) 管道进行有序构建
func (s *Server) buildInterceptors() ([]grpc.UnaryServerInterceptor, []grpc.StreamServerInterceptor) {
	pipeline := interceptors.NewServerPipeline(s.config.AuthToken)
	return pipeline.Build()
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
	if err := s.config.Validate(); err != nil {
		return nil, err
	}
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
	s.started.Store(true)

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
	if s.registry == nil {
		return nil
	}
	s.logger.Info("注册服务到 etcd",
		elog.String("serviceID", s.serviceID),
		elog.String("serviceName", s.ServiceName),
		elog.String("addr", addr))

	// NOTE: 使用 registry.Registry 接口注册服务,租约管理由 Registry 内部处理
	return s.registry.Register(context.Background(), registry.ServiceInstance{
		ID:       s.serviceID,
		Name:     s.ServiceName,
		Address:  addr,
		Metadata: s.metadata,
	})
}

func (s *Server) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.stop(ctx, false)
}

// 以下方法实现 server.Server 接口，使其能被 ego 框架的 egoApp.Serve() 使用

// Name 实现 server.Server 接口
func (s *Server) Name() string {
	return s.ServiceName
}

// Init 实现 server.Server 接口
func (s *Server) Init() error {
	return s.config.Validate()
}

// Start 实现 server.Server 接口 (阻塞执行直到服务停止，遵循 ego 框架规范)
func (s *Server) Start() error {
	listener, err := s.startServer()
	if err != nil {
		return err
	}

	if serveErr := s.Server.Serve(listener); serveErr != nil && !errors.Is(serveErr, grpc.ErrServerStopped) {
		s.logger.Error("gRPC 服务器错误", elog.FieldErr(serveErr))
		return serveErr
	}

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
	return s.stop(ctx, true)
}

func (s *Server) stop(ctx context.Context, graceful bool) error {
	s.started.Store(false)
	if !graceful {
		s.Server.Stop()
		return s.unregister(ctx)
	}
	unregisterErr := s.unregister(ctx)
	done := make(chan struct{})
	go func() {
		s.Server.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
		return unregisterErr
	case <-ctx.Done():
		// listener 已关闭；调用方收到 ctx 错误后负责取消仍在执行的业务任务。
		return errors.Join(unregisterErr, ctx.Err())
	}
}

func (s *Server) unregister(ctx context.Context) error {
	if s.registry == nil || s.registeredAddr == "" {
		return nil
	}
	err := s.registry.UnRegister(ctx, registry.ServiceInstance{
		ID: s.serviceID, Name: s.ServiceName, Address: s.registeredAddr,
	})
	if err != nil {
		s.logger.Error("注销服务失败", elog.FieldErr(err))
	}
	return err
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
	info.Healthy = s.started.Load()
	return &info
}
