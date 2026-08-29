package pool

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	agentDomain "github.com/Duke1616/etask/internal/agent/domain"
	"github.com/Duke1616/etask/internal/domain"
	poolSource "github.com/Duke1616/etask/internal/service/pool/syncer/source"
	"github.com/Duke1616/etask/pkg/grpc/registry"
	registryEtcd "github.com/Duke1616/etask/pkg/grpc/registry/etcd"
	"github.com/samber/lo"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const listNodesTimeout = 3 * time.Second

func (s *catalogService) ListNodes(ctx context.Context, pool domain.ExecutionPool) ([]Node, error) {
	nodes, err := s.ListNodesForPools(ctx, []domain.ExecutionPool{pool})
	if err != nil {
		return nil, err
	}
	return nodes[pool.Name], nil
}

// ListNodesForPools 每种注册前缀只查询一次，避免资源目录列表的 N+1 etcd 请求。
func (s *catalogService) ListNodesForPools(ctx context.Context, pools []domain.ExecutionPool) (map[string][]Node, error) {
	result := make(map[string][]Node, len(pools))
	if s.etcd == nil {
		return result, nil
	}

	queryCtx, cancel := context.WithTimeout(ctx, listNodesTimeout)
	defer cancel()

	poolsByKind := lo.GroupBy(pools, func(pool domain.ExecutionPool) domain.ExecutionPoolKind {
		return pool.Kind
	})

	for kind, poolList := range poolsByKind {
		prefix := nodePrefix(domain.ExecutionPool{Kind: kind})
		if prefix == "" {
			continue
		}
		resp, err := s.etcd.Get(queryCtx, prefix, clientv3.WithPrefix())
		if err != nil {
			return nil, err
		}
		poolsByName := lo.KeyBy(poolList, func(p domain.ExecutionPool) string {
			return p.Name
		})
		for _, kv := range resp.Kvs {
			inst, ok := poolSource.DecodeInstance(kv)
			if !ok {
				continue
			}
			poolName := instancePoolName(kind, inst)
			pool, exists := poolsByName[poolName]
			if !exists || !matchPoolInstance(pool, inst) {
				continue
			}
			id := strings.TrimSpace(inst.ID)
			if id == "" {
				id = strings.TrimSpace(inst.Address)
			}
			result[poolName] = append(result[poolName], Node{ID: id, Address: strings.TrimSpace(inst.Address)})
		}
	}

	for poolName := range result {
		slices.SortFunc(result[poolName], func(a, b Node) int {
			if a.ID == b.ID {
				return strings.Compare(a.Address, b.Address)
			}
			return strings.Compare(a.ID, b.ID)
		})
	}
	return result, nil
}

func instancePoolName(kind domain.ExecutionPoolKind, inst registry.ServiceInstance) string {
	switch kind {
	case domain.ExecutionPoolKindExecutor:
		if metadataString(inst.Metadata, "role") != "executor" {
			return ""
		}
		return strings.TrimSpace(inst.Name)
	case domain.ExecutionPoolKindAgent:
		return agentPoolName(inst.Name, inst.Metadata)
	default:
		return ""
	}
}

func nodePrefix(pool domain.ExecutionPool) string {
	switch pool.Kind {
	case domain.ExecutionPoolKindExecutor:
		if pool.Name == "" {
			return registryEtcd.DefaultPrefix + "/"
		}
		return path.Join(registryEtcd.DefaultPrefix, pool.Name) + "/"
	case domain.ExecutionPoolKindAgent:
		return "/etask/kafka/" + agentDomain.ServiceName
	default:
		return ""
	}
}

func matchPoolInstance(pool domain.ExecutionPool, inst registry.ServiceInstance) bool {
	switch pool.Kind {
	case domain.ExecutionPoolKindExecutor:
		return strings.TrimSpace(inst.Name) == pool.Name && metadataString(inst.Metadata, "role") == "executor"
	case domain.ExecutionPoolKindAgent:
		return agentPoolName(inst.Name, inst.Metadata) == pool.Name
	default:
		return false
	}
}

func agentPoolName(name string, metadata map[string]any) string {
	if val := metadataString(metadata, "name"); val != "" {
		return val
	}
	return strings.TrimSpace(name)
}

func metadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	if val, ok := metadata[key]; ok {
		return strings.TrimSpace(fmt.Sprintf("%v", val))
	}
	return ""
}
