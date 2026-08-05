package ioc

import (
	"github.com/Duke1616/etask/pkg/grpc/registry"
	"github.com/Duke1616/etask/pkg/grpc/registry/etcd"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// InitRegistry 初始化统一的服务注册中心
// 所有服务(Scheduler、Agent 等)都注册到 service/ 前缀下
// 通过 serviceName 区分不同服务: service/scheduler, service/cmdb, service/ticket 等
func InitRegistry(etcdClient *clientv3.Client) registry.Registry {
	r, err := etcd.NewRegistry(etcdClient)
	if err != nil {
		panic(err)
	}
	return r
}
