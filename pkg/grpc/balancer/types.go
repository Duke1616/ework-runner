package balancer

import "context"

// RoutingRoundRobinName 是路由式轮询负载均衡器的名称（支持排除+指定两种路由策略）
const RoutingRoundRobinName = "routing_round_robin"

// contextKey 是用于在 context 中传递排除节点信息的 key 类型
type contextKey string

// ExcludedNodeIDContextKey 是在 context 中存储要排除的节点 ID 的 key
const ExcludedNodeIDContextKey contextKey = "excluded_node_id"

// SpecificNodeIDContextKey 是在 context 中存储要指定的节点 ID 的 key
const SpecificNodeIDContextKey contextKey = "specific_node_id"

// WithExcludedNodeID 在 context 中设置要排除的节点 ID
func WithExcludedNodeID(ctx context.Context, nodeID string) context.Context {
	if nodeID == "" {
		return ctx
	}
	return context.WithValue(ctx, ExcludedNodeIDContextKey, nodeID)
}

// GetExcludeNode 从 context 中获取要排除的节点 ID
func GetExcludeNode(ctx context.Context) (string, bool) {
	nodeID, ok := ctx.Value(ExcludedNodeIDContextKey).(string)
	return nodeID, ok && nodeID != ""
}

// WithSpecificNodeID 在 context 中设置要指定的节点 ID
func WithSpecificNodeID(ctx context.Context, nodeID string) context.Context {
	if nodeID == "" {
		return ctx
	}
	return context.WithValue(ctx, SpecificNodeIDContextKey, nodeID)
}

// GetSpecificNodeID 从 context 中获取要指定的节点 ID
func GetSpecificNodeID(ctx context.Context) (string, bool) {
	nodeID, ok := ctx.Value(SpecificNodeIDContextKey).(string)
	return nodeID, ok && nodeID != ""
}
