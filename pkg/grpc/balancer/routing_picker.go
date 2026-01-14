package balancer

import (
	"sync/atomic"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// routingPicker 实现路由式轮询的 Picker（支持排除+指定两种路由策略）
type routingPicker struct {
	// subConns 是所有可用的子连接
	subConns []balancer.SubConn
	// nodeIDs 是与 subConns 对应的节点 ID
	nodeIDs []string
	// next 用于轮询的计数器
	next uint32
}

// newRoutingPicker 创建新的 routingPicker
func newRoutingPicker(subConns []balancer.SubConn, nodeIDs []string) *routingPicker {
	return &routingPicker{
		subConns: subConns,
		nodeIDs:  nodeIDs,
	}
}

// Pick 实现 balancer.Picker 接口，选择一个可用的连接
func (p *routingPicker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	if len(p.subConns) == 0 {
		return balancer.PickResult{}, balancer.ErrNoSubConnAvailable
	}

	// 优先级1：检查是否有指定节点ID（优先级最高）
	specificNodeID, hasSpecific := GetSpecificNodeID(info.Ctx)
	if hasSpecific {
		// 查找指定的节点
		for i := range p.nodeIDs {
			if p.nodeIDs[i] == specificNodeID {
				return balancer.PickResult{SubConn: p.subConns[i], Done: nil}, nil
			}
		}
		// 如果指定的节点不可用，返回错误
		return balancer.PickResult{}, status.Errorf(codes.Unavailable,
			"指定的节点不可用: %s", specificNodeID)
	}

	// 优先级2：检查排除节点ID
	excludeNodeID, hasExclude := GetExcludeNode(info.Ctx)

	// 如果没有排除节点，或者只有一个连接，直接使用轮询
	if !hasExclude || len(p.subConns) == 1 {
		return p.pickRoundRobin(), nil
	}

	// 找出所有可用的 candidate 的索引
	candidateIndexes := make([]int, 0, len(p.subConns))
	for i, nodeID := range p.nodeIDs {
		if nodeID != excludeNodeID {
			candidateIndexes = append(candidateIndexes, i)
		}
	}

	// 如果过滤后没有可用连接
	if len(candidateIndexes) == 0 {
		// 如果所有连接都被排除了，作为最后的手段，可以忽略排除规则，任选一个
		// 或者返回错误，取决于业务需求
		if len(p.subConns) > 0 {
			return p.pickRoundRobin(), nil
		}
		return balancer.PickResult{}, status.Errorf(codes.Unavailable,
			"所有可用节点都被排除，排除节点: %s", excludeNodeID)
	}

	// 在可用的索引中进行轮询
	next := atomic.AddUint32(&p.next, 1)
	selectedIdxInCandidates := int(next-1) % len(candidateIndexes)
	selectedConn := p.subConns[candidateIndexes[selectedIdxInCandidates]]
	return balancer.PickResult{SubConn: selectedConn, Done: nil}, nil
}

// pickRoundRobin 执行标准的轮询选择
func (p *routingPicker) pickRoundRobin() balancer.PickResult {
	next := atomic.AddUint32(&p.next, 1)
	idx := int(next-1) % len(p.subConns)

	return balancer.PickResult{
		SubConn: p.subConns[idx],
		Done: func(_ balancer.DoneInfo) {
			// 可以在这里添加请求完成后的处理逻辑
		},
	}
}
