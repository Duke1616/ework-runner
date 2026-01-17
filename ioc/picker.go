package ioc

import (
	"github.com/Duke1616/ework-runner/internal/service/picker"
	"github.com/Duke1616/ework-runner/pkg/grpc/registry/etcd"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// InitExecutorNodePicker 初始化执行节点选择器
func InitExecutorNodePicker(etcdClient *clientv3.Client) picker.ExecutorNodePicker {
	// NOTE: Executor 注册在默认前缀 /services/etask/executor 下
	executorReg, err := etcd.NewRegistry(etcdClient)
	if err != nil {
		panic(err)
	}

	return picker.NewRandomPicker(executorReg)
}
