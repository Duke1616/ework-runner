package grpc

import (
	"context"
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
	L           *elog.Component
}

// NewServer 创建 gRPC Server 实例
// NOTE: 当前此构造函数创建的 Server 需要外部配置 EtcdClient、Port、EtcdTTL
// 如果需要完整配置，建议使用配置文件方式或手动设置字段
func NewServer(name, addr string, etcdClient *clientv3.Client) *Server {
	return &Server{
		Server:      grpc.NewServer(),
		ServiceName: name,
		EtcdClient:  etcdClient,
		Port:        extractPort(addr),
		EtcdTTL:     10, // 默认 10 秒
		L:           elog.DefaultLogger.With(elog.FieldComponentName("grpc.server")),
	}
}

// extractPort 从地址中提取端口号
func extractPort(addr string) int {
	// 支持 ":8080" 或 "0.0.0.0:8080" 格式
	if len(addr) > 0 && addr[0] == ':' {
		port, _ := strconv.Atoi(addr[1:])
		return port
	}
	// 查找最后一个 ':'
	idx := len(addr) - 1
	for idx >= 0 && addr[idx] != ':' {
		idx--
	}
	if idx >= 0 && idx+1 < len(addr) {
		port, _ := strconv.Atoi(addr[idx+1:])
		return port
	}
	return 8080 // 默认端口
}

// Serve 启动服务器并且阻塞
func (s *Server) Serve() error {
	// 初始化一个控制整个过程的 ctx
	// 你也可以考虑让外面传进来，这样的话就是 main 函数自己去控制了
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	port := strconv.Itoa(s.Port)
	l, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}
	// 要先确保启动成功，再注册服务
	err = s.register(ctx, port)
	if err != nil {
		return err
	}
	return s.Server.Serve(l)
}

func (s *Server) register(ctx context.Context, port string) error {
	cli := s.EtcdClient
	serviceName := "service/" + s.ServiceName
	em, err := endpoints.NewManager(cli,
		serviceName)
	if err != nil {
		return err
	}
	s.etcdManager = em
	ip := netx.GetOutboundIP()
	s.etcdKey = serviceName + "/" + ip
	addr := ip + ":" + port
	leaseResp, err := cli.Grant(ctx, s.EtcdTTL)
	// 开启续约
	ch, err := cli.KeepAlive(ctx, leaseResp.ID)
	if err != nil {
		return err
	}
	go func() {
		// 可以预期，当我们的 cancel 被调用的时候，就会退出这个循环
		for chResp := range ch {
			s.L.Debug("续约：", elog.String("resp", chResp.String()))
		}
	}()

	// metadata 我们这里没啥要提供的
	return em.AddEndpoint(ctx, s.etcdKey,
		endpoints.Endpoint{Addr: addr}, clientv3.WithLease(leaseResp.ID))
}

func (s *Server) Close() error {
	s.cancel()
	if s.etcdManager != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		err := s.etcdManager.DeleteEndpoint(ctx, s.etcdKey)
		if err != nil {
			return err
		}
	}
	err := s.EtcdClient.Close()
	if err != nil {
		return err
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
	// 初始化控制上下文
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	port := strconv.Itoa(s.Port)
	l, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}

	// 注册服务到 etcd
	err = s.register(ctx, port)
	if err != nil {
		l.Close()
		return err
	}

	// 异步启动 gRPC 服务
	go func() {
		if err = s.Server.Serve(l); err != nil {
			s.L.Error("gRPC 服务器错误", elog.FieldErr(err))
		}
	}()

	return nil
}

// Stop 实现 server.Server 接口
func (s *Server) Stop() error {
	s.L.Info("停止 gRPC 服务器")
	return s.Close()
}

// GracefulStop 实现 server.Server 接口
func (s *Server) GracefulStop(ctx context.Context) error {
	s.L.Info("优雅停止 gRPC 服务器")

	// 先注销服务
	if s.etcdManager != nil {
		if err := s.etcdManager.DeleteEndpoint(ctx, s.etcdKey); err != nil {
			s.L.Error("注销服务失败", elog.FieldErr(err))
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
	return "grpc.server"
}

// Info 实现 server.Server 接口
func (s *Server) Info() *server.ServiceInfo {
	info := server.ApplyOptions(
		server.WithName(s.ServiceName),
		server.WithKind(constant.ServiceProvider),
		server.WithScheme("grpc"),
		server.WithAddress(":"+strconv.Itoa(s.Port)),
	)

	// 判断服务是否健康（检查 cancel 是否被调用）
	info.Healthy = s.cancel != nil
	return &info
}
