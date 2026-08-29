package source

import (
	"context"
	"strings"

	agentdomain "github.com/Duke1616/etask/internal/agent/domain"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/pkg/grpc/registry"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const agentServicePrefix = "/etask/kafka/" + agentdomain.ServiceName

type agentSource struct {
	etcd *clientv3.Client
}

func NewAgent(etcd *clientv3.Client) Source {
	return agentSource{etcd: etcd}
}

func (agentSource) Name() string {
	return "agent"
}

func (agentSource) Prefix() string {
	return agentServicePrefix
}

func (agentSource) Kind() domain.ExecutionPoolKind {
	return domain.ExecutionPoolKindAgent
}

func (agentSource) Accept(registry.ServiceInstance) bool {
	return true
}

func (agentSource) BuildPool(inst registry.ServiceInstance) (domain.ExecutionPool, bool) {
	return buildAgentPool(inst)
}

func (agentSource) PoolName(inst registry.ServiceInstance) string {
	return agentPoolName(inst)
}

func (s agentSource) HasInstances(ctx context.Context, poolName string) (bool, error) {
	return hasInstance(ctx, s.etcd, agentServicePrefix, func(inst registry.ServiceInstance) bool {
		return agentPoolName(inst) == poolName
	})
}

func buildAgentPool(inst registry.ServiceInstance) (domain.ExecutionPool, bool) {
	name := agentPoolName(inst)
	if name == "" {
		return domain.ExecutionPool{}, false
	}
	return newPool(name, domain.ExecutionPoolKindAgent, domain.ExecutionTransportMQ,
		domain.ExecModePush, inst), true
}

func agentPoolName(inst registry.ServiceInstance) string {
	name := metadataString(inst.Metadata, "name")
	if name == "" {
		name = inst.Name
	}
	return strings.TrimSpace(name)
}
