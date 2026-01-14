package ioc

import (
	registry "github.com/Duke1616/ecmdb/pkg/grpc/registry/etcd"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func InitRegistry(etcdClient *clientv3.Client) *registry.Registry {
	r, err := registry.NewRegistry(etcdClient)
	if err != nil {
		panic(err)
	}
	return r
}
