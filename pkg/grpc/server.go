package grpc

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/Duke1616/ework-runner/pkg/netx"
	"github.com/gotomicro/ego/core/constant"
	"github.com/gotomicro/ego/core/elog"
	"github.com/gotomicro/ego/server"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/naming/endpoints"
	"google.golang.org/grpc"
)

const (
	// ServicePrefix 服务注册路径前缀
	ServicePrefix = "service/"
	// ComponentName 日志组件名
	ComponentName = "grpc.server"
	// DefaultPort 默认端口
	DefaultPort = 8080
	// DefaultTTL 默认租约TTL(秒)
	DefaultTTL = 10
)

type Server struct {
	*grpc.Server
	Port int
	// ETCD 服务注册租约 TTL
	EtcdTTL     int64
	EtcdClient  *clientv3.Client
	etcdManager endpoints.Manager
	etcdKey     string
	cancel      func()
	ServiceName string
	logger      *elog.Component
}

// NewServer 创建 gRPC Server 实例
func NewServer(name, addr string, etcdClient *clientv3.Client) *Server {
	return &Server{
		Server:      grpc.NewServer(),
		ServiceName: name,
		EtcdClient:  etcdClient,
		Port:        extractPort(addr),
		EtcdTTL:     DefaultTTL,
		logger:      elog.DefaultLogger.With(elog.FieldComponentName(ComponentName)),
	}
}

// extractPort 从地址中提取端口号
func extractPort(addr string) int {
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		// 可能是 ":8080" 格式
		if len(addr) > 0 && addr[0] == ':' {
			if port, err := strconv.Atoi(addr[1:]); err == nil {
				return port
			}
		}
		return DefaultPort
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return DefaultPort
	}
	return port
}

// startServer 启动服务器并注册到 etcd (内部方法)
func (s *Server) startServer() (net.Listener, error) {
	// 初始化控制上下文
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	port := strconv.Itoa(s.Port)
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return nil, fmt.Errorf("监听端口失败: %w", err)
	}

	// 注册服务到 etcd
	if err := s.register(ctx, port); err != nil {
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

func (s *Server) register(ctx context.Context, port string) error {
	cli := s.EtcdClient
	serviceName := ServicePrefix + s.ServiceName

	em, err := endpoints.NewManager(cli, serviceName)
	if err != nil {
		return fmt.Errorf("创建 endpoint manager 失败: %w", err)
	}
	s.etcdManager = em

	ip := netx.GetOutboundIP()
	s.etcdKey = serviceName + "/" + ip
	addr := ip + ":" + port

	// 创建租约
	leaseResp, err := cli.Grant(ctx, s.EtcdTTL)
	if err != nil {
		return fmt.Errorf("创建租约失败: %w", err)
	}

	// 开启续约
	ch, err := cli.KeepAlive(ctx, leaseResp.ID)
	if err != nil {
		return fmt.Errorf("开启租约续约失败: %w", err)
	}

	go func() {
		// 当 cancel 被调用时,会退出此循环
		for chResp := range ch {
			s.logger.Debug("续约", elog.String("resp", chResp.String()))
		}
	}()

	// 添加服务端点
	return em.AddEndpoint(ctx, s.etcdKey,
		endpoints.Endpoint{Addr: addr}, clientv3.WithLease(leaseResp.ID))
}

func (s *Server) Close() error {
	if s.cancel != nil {
		s.cancel()
	}

	if s.etcdManager != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := s.etcdManager.DeleteEndpoint(ctx, s.etcdKey); err != nil {
			s.logger.Error("注销服务失败", elog.FieldErr(err))
		}
	}

	if s.EtcdClient != nil {
		if err := s.EtcdClient.Close(); err != nil {
			return err
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
		if err := s.Server.Serve(listener); err != nil {
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

	// 先注销服务
	if s.etcdManager != nil {
		if err := s.etcdManager.DeleteEndpoint(ctx, s.etcdKey); err != nil {
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
		server.WithAddress(":"+strconv.Itoa(s.Port)),
	)

	// 判断服务是否健康
	info.Healthy = s.cancel != nil
	return &info
}
