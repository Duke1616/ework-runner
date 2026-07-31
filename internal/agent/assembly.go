package agent

import (
	"encoding/json"
	"fmt"
	"time"

	artifactv1 "github.com/Duke1616/etask/api/proto/gen/etask/artifact/v1"
	"github.com/Duke1616/etask/internal/agent/domain"
	"github.com/Duke1616/etask/internal/agent/event"
	"github.com/Duke1616/etask/internal/agent/service"
	"github.com/Duke1616/etask/internal/grpc/scripts"
	config "github.com/Duke1616/etask/pkg/config"
	grpcpkg "github.com/Duke1616/etask/pkg/grpc"
	"github.com/Duke1616/etask/pkg/grpc/registry"
	"github.com/Duke1616/etask/pkg/grpc/registry/etcd"
	"github.com/Duke1616/etask/sdk/executor"
	"github.com/ecodeclub/mq-api"
	"github.com/google/uuid"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
)

type Instance struct {
	Name           string `yaml:"name" json:"name"`                       // 实例名称
	Desc           string `yaml:"desc" json:"desc"`                       // 注解
	Topic          string `yaml:"topic" json:"topic"`                     // 建立 Topic 通道
	WorkerCount    int    `yaml:"worker_count" json:"worker_count"`       // 并发工作协程数量
	IsolationLevel string `yaml:"isolation_level" json:"isolation_level"` // 资源池隔离级别: SHARED 或 DEDICATED
}

func InitModule(q mq.MQ, etcdClient *clientv3.Client,
	preparer executor.ArtifactPreparer, scriptRuntime *scripts.Runtime) *Module {
	registry := InitRegistry(etcdClient)
	if err := scriptRuntime.Initialize(); err != nil {
		panic(err)
	}
	connection := initSchedulerConnection(etcdClient)
	artifactClient := artifactv1.NewArtifactServiceClient(connection)
	executionService := service.NewService(scriptRuntime.Handlers(), preparer, artifactClient)
	publisher := initExecutionEventPublisher(q)
	consumer := initExecuteConsumer(q, executionService, publisher, registry)
	return &Module{
		Svc: executionService, C: consumer, connection: connection, artifacts: preparer,
	}
}

func initExecutionEventPublisher(q mq.MQ) event.ExecutionEventPublisher {
	publisher, err := event.NewExecutionEventPublisher(q)
	if err != nil {
		panic(err)
	}
	return publisher
}

func InitRegistry(etcdClient *clientv3.Client) registry.Registry {
	reg, err := etcd.NewRegistryWithPrefix(etcdClient, "/etask/kafka")
	if err != nil {
		panic(err)
	}
	return reg
}

func initExecuteConsumer(q mq.MQ, svc service.Service, publisher event.ExecutionEventPublisher,
	reg registry.Registry) *event.ExecuteConsumer {
	var cfg Instance
	if err := config.UnmarshalKey("agent", &cfg); err != nil {
		panic(err)
	}

	handlerMetas, _ := json.Marshal(svc.ListHandlers())
	instance := registry.ServiceInstance{
		Name:    domain.ServiceName, // 统一的服务分组名称
		Address: uuid.New().String(),
		Metadata: map[string]any{
			"name":               cfg.Name,
			"desc":               cfg.Desc,
			"topic":              cfg.Topic,
			"supported_handlers": string(handlerMetas),
			"isolation_level":    cfg.IsolationLevel,
		},
	}

	consumer, err := event.NewExecuteConsumer(q, svc, cfg.Topic, publisher, reg, instance, cfg.WorkerCount)
	if err != nil {
		panic(err)
	}

	return consumer
}

func initSchedulerConnection(etcdClient *clientv3.Client) *grpc.ClientConn {
	var configVal grpcpkg.ClientConfig
	if err := config.UnmarshalKey("grpc.client.scheduler", &configVal); err != nil {
		panic(err)
	}
	connection, err := schedulerConnection(configVal, etcdClient)
	if err != nil {
		panic(fmt.Errorf("创建 Agent 调度中心连接失败: %w", err))
	}
	return connection
}

func schedulerConnection(config grpcpkg.ClientConfig, etcdClient *clientv3.Client) (*grpc.ClientConn, error) {
	options := []grpcpkg.ClientOption{
		grpcpkg.WithClientJWTAuth(config.AuthToken),
		grpcpkg.WithTimeout(10 * time.Second),
	}
	if config.Address != "" {
		return grpcpkg.NewDirectClientConn(config.Address, options...)
	}
	registry, err := etcd.NewRegistry(etcdClient)
	if err != nil {
		return nil, err
	}
	return grpcpkg.NewClientConn(registry, append(options, grpcpkg.WithServiceName(config.Name))...)
}
