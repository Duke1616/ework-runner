package source

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/pkg/grpc/registry"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const instanceCheckTimeout = 5 * time.Second

// Source 描述一种可以被同步为 execution_pools 的注册中心资源来源。
type Source interface {
	// Name 返回来源的唯一名称。
	Name() string
	// Prefix 返回来源在注册中心监听的键前缀。
	Prefix() string
	// Kind 返回来源对应的执行资源池类型。
	Kind() domain.ExecutionPoolKind
	// Accept 判断注册实例是否属于当前来源。
	Accept(registry.ServiceInstance) bool
	// BuildPool 将注册实例转换为执行资源池。
	BuildPool(registry.ServiceInstance) (domain.ExecutionPool, bool)
	// PoolName 返回注册实例所属的资源池名称。
	PoolName(registry.ServiceInstance) string
	// HasInstances 判断指定资源池是否仍有在线实例。
	HasInstances(context.Context, string) (bool, error)
}

func newPool(name string, kind domain.ExecutionPoolKind, transport domain.ExecutionTransport,
	dispatchMode domain.ExecMode, inst registry.ServiceInstance) domain.ExecutionPool {
	return domain.ExecutionPool{
		Name:           name,
		Kind:           kind,
		Transport:      transport,
		DispatchMode:   dispatchMode,
		IsolationLevel: normalizeIsolationLevel(inst),
		Desc:           metadataString(inst.Metadata, "desc"),
		Status:         domain.ExecutionPoolStatusEnabled,
		Metadata:       metadataColumn(inst),
	}
}

// normalizeIsolationLevel 从注册元数据读取资源池隔离级别；缺省值为 SHARED。
// 注册协议使用 metadata.isolation_level，支持 SHARED 和 DEDICATED。
func normalizeIsolationLevel(inst registry.ServiceInstance) domain.ExecutionPoolIsolation {
	switch strings.ToUpper(metadataString(inst.Metadata, "isolation_level")) {
	case domain.ExecutionPoolIsolationDedicated.String():
		return domain.ExecutionPoolIsolationDedicated
	case domain.ExecutionPoolIsolationShared.String(), "":
		return domain.ExecutionPoolIsolationShared
	default:
		return domain.ExecutionPoolIsolationShared
	}
}

func DecodeInstance(kv *mvccpb.KeyValue) (registry.ServiceInstance, bool) {
	var inst registry.ServiceInstance
	if kv == nil || len(kv.Value) == 0 {
		return inst, false
	}
	if err := json.Unmarshal(kv.Value, &inst); err != nil {
		return inst, false
	}
	return inst, true
}

func hasInstance(ctx context.Context, etcd *clientv3.Client, prefix string, match func(registry.ServiceInstance) bool) (bool, error) {
	checkCtx, cancel := context.WithTimeout(ctx, instanceCheckTimeout)
	defer cancel()

	resp, err := etcd.Get(checkCtx, prefix, clientv3.WithPrefix())
	if err != nil {
		return false, err
	}
	for _, kv := range resp.Kvs {
		inst, ok := DecodeInstance(kv)
		if ok && match(inst) {
			return true, nil
		}
	}
	return false, nil
}

func metadataColumn(inst registry.ServiceInstance) map[string]string {
	metadata := make(map[string]string, len(inst.Metadata)+4)
	metadata["managed_by"] = "registry"
	metadata["instance_id"] = inst.ID
	metadata["address"] = inst.Address
	metadata["registry_name"] = inst.Name
	for key, val := range inst.Metadata {
		metadata[key] = fmt.Sprintf("%v", val)
	}
	return metadata
}

func IsRegistryManaged(pool domain.ExecutionPool) bool {
	return pool.Metadata != nil && pool.Metadata["managed_by"] == "registry"
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
