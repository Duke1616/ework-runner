package source

import (
	"context"
	"path"
	"strings"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/pkg/grpc/registry"
	registryetcd "github.com/Duke1616/etask/pkg/grpc/registry/etcd"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type executorSource struct {
	etcd *clientv3.Client
}

func NewExecutor(etcd *clientv3.Client) Source {
	return executorSource{etcd: etcd}
}

func (executorSource) Name() string {
	return "executor"
}

func (executorSource) Prefix() string {
	return registryetcd.DefaultPrefix
}

func (executorSource) Kind() domain.ExecutionPoolKind {
	return domain.ExecutionPoolKindExecutor
}

func (executorSource) Accept(inst registry.ServiceInstance) bool {
	return isExecutorInstance(inst)
}

func (executorSource) BuildPool(inst registry.ServiceInstance) (domain.ExecutionPool, bool) {
	return buildExecutorPool(inst)
}

func (executorSource) PoolName(inst registry.ServiceInstance) string {
	return executorPoolName(inst)
}

func (s executorSource) HasInstances(ctx context.Context, poolName string) (bool, error) {
	keyPrefix := path.Join(registryetcd.DefaultPrefix, poolName) + "/"
	return hasInstance(ctx, s.etcd, keyPrefix, isExecutorInstance)
}

func buildExecutorPool(inst registry.ServiceInstance) (domain.ExecutionPool, bool) {
	if !isExecutorInstance(inst) {
		return domain.ExecutionPool{}, false
	}

	name := executorPoolName(inst)
	if name == "" {
		return domain.ExecutionPool{}, false
	}

	return newPool(name, domain.ExecutionPoolKindExecutor, domain.ExecutionTransportGRPC,
		normalizeExecutorMode(inst), inst), true
}

func executorPoolName(inst registry.ServiceInstance) string {
	return strings.TrimSpace(inst.Name)
}

func isExecutorInstance(inst registry.ServiceInstance) bool {
	return metadataString(inst.Metadata, "role") == "executor"
}

func normalizeExecutorMode(inst registry.ServiceInstance) domain.ExecMode {
	switch strings.ToUpper(metadataString(inst.Metadata, "mode")) {
	case domain.ExecModePull.String():
		return domain.ExecModePull
	case domain.ExecModePush.String(), "":
		return domain.ExecModePush
	default:
		return domain.ExecModePush
	}
}
