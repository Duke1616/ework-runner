package balancer

import (
	"sync"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/resolver"
)

// routingBalancer 实现路由式负载均衡器（支持排除+指定两种路由策略）
type routingBalancer struct {
	// cc 用于向 gRPC 报告 Picker 的更新
	cc balancer.ClientConn
	mu sync.RWMutex

	subConnMap map[string]balancer.SubConn
	// scToAddrMap 维护从gRPC子连接到解析器地址的反向映射
	scToAddrMap map[balancer.SubConn]string
	// nodeIDMap 维护从解析器地址到业务节点ID的映射
	nodeIDMap map[string]string
	// readySCs 维护了所有处于 READY 状态的子连接及其节点ID
	// 这是构建 Picker 的唯一数据源，确保了只有健康的连接会被选中
	readySCs map[balancer.SubConn]string
}

// newRoutingBalancer 创建新的排除式负载均衡器
func newRoutingBalancer(cc balancer.ClientConn) *routingBalancer {
	return &routingBalancer{
		cc:          cc,
		subConnMap:  make(map[string]balancer.SubConn),
		scToAddrMap: make(map[balancer.SubConn]string),
		nodeIDMap:   make(map[string]string),
		readySCs:    make(map[balancer.SubConn]string),
	}
}

// UpdateClientConnState 在服务发现的地址列表发生变化时被调用
func (b *routingBalancer) UpdateClientConnState(state balancer.ClientConnState) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 将新的地址列表转换成 map，方便快速查找
	newAddrs := make(map[string]resolver.Address)
	for _, addr := range state.ResolverState.Addresses {
		newAddrs[addr.Addr] = addr
	}

	// 移除不再存在的连接
	for addrStr, sc := range b.subConnMap {
		if _, ok := newAddrs[addrStr]; !ok {
			// 先从 Picker 数据源移除，再关闭连接；Shutdown 回调可能在映射清理后到达。
			delete(b.subConnMap, addrStr)
			delete(b.scToAddrMap, sc)
			delete(b.nodeIDMap, addrStr)
			delete(b.readySCs, sc)
			sc.Shutdown()
		}
	}

	// 添加新的连接
	for addrStr, addr := range newAddrs {
		if sc, ok := b.subConnMap[addrStr]; ok {
			// 地址已存在，尝试唤醒或重连（针对 Idle 或 TransientFailure 的子连接）
			// 这能保证在断网恢复且地址没变的情况下，不会干等原本的长 backoff！
			sc.Connect()
			continue
		}

		// 为新地址创建子连接
		sc, err := b.cc.NewSubConn([]resolver.Address{addr}, balancer.NewSubConnOptions{})
		if err != nil {
			// 在实践中应该记录这个错误
			continue
		}
		// 维护正向和反向映射
		b.subConnMap[addrStr] = sc
		b.scToAddrMap[sc] = addrStr // 添加反向映射
		b.nodeIDMap[addrStr] = b.extractNodeID(addr)
		// 开始连接，这会异步触发 UpdateSubConnState 的调
		sc.Connect()
	}

	// 地址删除必须立即刷新 Picker；新连接仍由后续状态回调加入 readySCs。
	b.updatePicker()
	return nil
}

// UpdateSubConnState 在子连接的状态发生变化时被调用
func (b *routingBalancer) UpdateSubConnState(sc balancer.SubConn, state balancer.SubConnState) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 使用反向映射快速查找子连接对应的地址
	addrStr, ok := b.scToAddrMap[sc]
	if !ok {
		// 如果反向映射中没有找到，说明该连接可能已被移除，直接忽略
		return
	}

	switch state.ConnectivityState {
	case connectivity.Ready:
		// 连接就绪，将其添加到可用连接列表
		b.readySCs[sc] = b.nodeIDMap[addrStr]
	case connectivity.Idle, connectivity.Connecting, connectivity.TransientFailure:
		// 连接不可用，从可用连接列表中移除
		delete(b.readySCs, sc)
	case connectivity.Shutdown:
		// 连接已关闭，从所有记录中彻底移除
		delete(b.subConnMap, addrStr)
		delete(b.scToAddrMap, sc) // 清理反向映射
		delete(b.nodeIDMap, addrStr)
		delete(b.readySCs, sc)
	}

	// 每当可用连接集合可能发生变化时，都重新生成并更新 Picker
	b.updatePicker()
}

// updatePicker 根据当前可用的连接列表，创建新的 Picker 并更新客户端状态
// 这个函数必须在持有锁的情况下被调用
func (b *routingBalancer) updatePicker() {
	if len(b.readySCs) == 0 {
		// 没有可用的连接，通知客户端连接正在处理中（让外部阻塞等待，而不是立刻失败报错）
		b.cc.UpdateState(balancer.State{
			ConnectivityState: connectivity.Connecting,
			Picker:            base.NewErrPicker(balancer.ErrNoSubConnAvailable),
		})
		return
	}

	// 将可用的连接和节点ID从 map 转换成切片，以供 Picker 使用
	readyConns := make([]balancer.SubConn, 0, len(b.readySCs))
	nodeIDs := make([]string, 0, len(b.readySCs))
	for sc, nodeID := range b.readySCs {
		readyConns = append(readyConns, sc)
		nodeIDs = append(nodeIDs, nodeID)
	}

	// 创建新的 Picker，并更新客户端状态为就绪
	b.cc.UpdateState(balancer.State{
		ConnectivityState: connectivity.Ready,
		Picker:            newRoutingPicker(readyConns, nodeIDs),
	})
}

// extractNodeID 从地址的 attributes 中提取节点 ID
func (b *routingBalancer) extractNodeID(addr resolver.Address) string {
	// 服务发现机制必须在 resolver.Address.Attributes 中注入节点ID
	// 这里假设节点 ID 存储在 "nodeID" 字段
	if addr.Attributes != nil {
		if nodeIDVal := addr.Attributes.Value("nodeID"); nodeIDVal != nil {
			if nodeIDStr, ok := nodeIDVal.(string); ok {
				return nodeIDStr
			}
		}
	}
	// 如果没有找到节点 ID，使用地址作为兜底，确保每个连接都有一个唯一标识
	return addr.Addr
}

// ResolverError 在解析器发生错误时被调用
func (b *routingBalancer) ResolverError(error) {
	// 在实践中，应该记录这个错误。
	// gRPC 建议此时不要改变连接状态，等待解析器恢复。
}

// Close 关闭负载均衡器，释放所有资源
func (b *routingBalancer) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, sc := range b.subConnMap {
		sc.Shutdown()
	}
	// 清理所有映射关系
	b.subConnMap = nil
	b.scToAddrMap = nil // 清理反向映射
	b.nodeIDMap = nil
	b.readySCs = nil
}

// ExitIdle 在 balancer 处于 Idle 状态时被 gRPC 调用，通知其尝试重新连接。
func (b *routingBalancer) ExitIdle() {
	b.mu.RLock()
	defer b.mu.RUnlock()
	// NOTE: 这里是修复唤醒死锁的核心：要唤醒的是所有的 SubConn，而不是 readySCs 里的！
	for _, sc := range b.subConnMap {
		sc.Connect()
	}
}
