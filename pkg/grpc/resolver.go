package grpc

import (
	"context"
	"sync"
	"time"

	"github.com/Duke1616/etask/pkg/grpc/registry"
	"github.com/gotomicro/ego/core/elog"
	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/resolver"
)

const (
	initCapacityStr = "initCapacity"
	maxCapacityStr  = "maxCapacity"
	increaseStepStr = "increaseStep"
	growthRateStr   = "growthRate"
	nodeIDStr       = "nodeID"
)

type resolverBuilder struct {
	r       registry.Registry
	timeout time.Duration
}

func NewResolverBuilder(r registry.Registry, timeout time.Duration) resolver.Builder {
	return &resolverBuilder{
		r:       r,
		timeout: timeout,
	}
}

func (r *resolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, _ resolver.BuildOptions) (resolver.Resolver, error) {
	ctx, cancel := context.WithCancel(context.Background())
	res := &executorResolver{
		target:       target,
		cc:           cc,
		registry:     r.r,
		close:        make(chan struct{}),
		ctx:          ctx,
		cancel:       cancel,
		updateNotify: make(chan struct{}, 1),
		timeout:      r.timeout,
		logger:       elog.DefaultLogger.With(elog.FieldComponentName("grpc.resolver")),
	}
	go res.reconcileLoop()
	go res.watch()
	res.ResolveNow(resolver.ResolveNowOptions{})
	return res, nil
}

func (r *resolverBuilder) Scheme() string {
	return "executor"
}

type executorResolver struct {
	target        resolver.Target
	cc            resolver.ClientConn
	registry      registry.Registry
	close         chan struct{}
	closeOnce     sync.Once
	ctx           context.Context
	cancel        context.CancelFunc
	updateNotify  chan struct{}
	timeout       time.Duration
	lastAddresses []resolver.Address
	logger        *elog.Component
}

func (g *executorResolver) ResolveNow(_ resolver.ResolveNowOptions) {
	select {
	case g.updateNotify <- struct{}{}:
	default:
	}
}

func (g *executorResolver) Close() {
	g.closeOnce.Do(func() {
		g.cancel()
		close(g.close)
	})
}

func (g *executorResolver) watch() {
	var events <-chan registry.Event
	if subscriber, ok := g.registry.(registry.ContextSubscriber); ok {
		events = subscriber.SubscribeContext(g.ctx, g.target.Endpoint())
	} else {
		events = g.registry.Subscribe(g.target.Endpoint())
	}
	for {
		select {
		case _, ok := <-events:
			if !ok {
				return
			}
			g.ResolveNow(resolver.ResolveNowOptions{})
		case <-g.close:
			return
		}
	}
}

func (g *executorResolver) reconcileLoop() {
	for {
		select {
		case <-g.updateNotify:
			g.reconcile()
		case <-g.close:
			return
		}
	}
}

func (g *executorResolver) reconcile() {
	serviceName := g.target.Endpoint()
	ctx, cancel := context.WithTimeout(g.ctx, g.timeout)
	instances, err := g.registry.ListServices(ctx, serviceName)
	cancel()

	if err != nil {
		g.cc.ReportError(err)
		return
	}

	if len(instances) == 0 {
		g.logger.Warn("服务发现结果为空", elog.String("service", serviceName))
		return
	}

	address := make([]resolver.Address, 0, len(instances))
	for _, ins := range instances {
		address = append(address, resolver.Address{
			Addr:       ins.Address,
			ServerName: ins.Name,
			Attributes: attributes.New(initCapacityStr, ins.InitCapacity).
				WithValue(maxCapacityStr, ins.MaxCapacity).
				WithValue(increaseStepStr, ins.IncreaseStep).
				WithValue(growthRateStr, ins.GrowthRate).
				WithValue(nodeIDStr, ins.ID),
		})
	}

	// 总是更新状态，让 Balancer 有机会重置或唤醒 SubConn
	g.lastAddresses = address

	err = g.cc.UpdateState(resolver.State{
		Addresses: address,
	})
	if err != nil {
		g.logger.Error("更新 gRPC 状态失败", elog.FieldErr(err))
		g.cc.ReportError(err)
	}
}
