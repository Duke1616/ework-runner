package ioc

import (
	"github.com/Duke1616/ework-runner/pkg/grpc/registry"
	"github.com/Duke1616/ework-runner/pkg/grpc/registry/etcd"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func InitRegistry(etcdClient *clientv3.Client) registry.Registry {
	r, err := etcd.NewRegistry(etcdClient)
	if err != nil {
		panic(err)
	}
	return r
}
