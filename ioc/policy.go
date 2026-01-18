package ioc

import (
	policyv1 "github.com/Duke1616/ework-runner/api/proto/gen/policy/v1"
	"github.com/Duke1616/ework-runner/pkg/grpc/interceptors/jwt"
	"github.com/spf13/viper"
	etcdv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/naming/resolver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func InitECMDBGrpcClient(etcdClient *etcdv3.Client) grpc.ClientConnInterface {
	type Config struct {
		Target string `mapstructure:"target"`
		Secure bool   `mapstructure:"secure"`
		Key    string `mapstructure:"key"`
	}
	var cfg Config
	err := viper.UnmarshalKey("grpc.client.ecmdb", &cfg)
	if err != nil {
		panic(err)
	}

	rs, err := resolver.NewBuilder(etcdClient)
	if err != nil {
		panic(err)
	}

	// 创建 JWT 客户端拦截器
	// NOTE: biz_id 会从每次请求的 context 中动态获取
	jwtInterceptor := jwt.NewClientInterceptorBuilder(cfg.Key)

	opts := []grpc.DialOption{
		grpc.WithResolvers(rs),
		grpc.WithUnaryInterceptor(jwtInterceptor.UnaryClientInterceptor()),
	}
	if !cfg.Secure {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	cc, err := grpc.NewClient(cfg.Target, opts...)
	if err != nil {
		panic(err)
	}

	return cc
}

func InitPolicyServiceClient(cc grpc.ClientConnInterface) policyv1.PolicyServiceClient {
	return policyv1.NewPolicyServiceClient(cc)
}
