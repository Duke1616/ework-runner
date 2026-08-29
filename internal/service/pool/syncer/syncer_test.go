package syncer

import (
	"strings"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
)

func TestAggregatePoolInstances(t *testing.T) {
	testCases := []struct {
		name    string
		pools   []domain.ExecutionPool
		wantErr string
		want    string
	}{
		{
			name: "合并不同节点的 Handler 能力",
			pools: []domain.ExecutionPool{
				poolFixture(domain.ExecutionPoolKindExecutor, domain.ExecutionTransportGRPC, domain.ExecModePush, "", `[{"name":"shell"}]`),
				poolFixture(domain.ExecutionPoolKindExecutor, domain.ExecutionTransportGRPC, domain.ExecModePush, "", `[{"name":"python"}]`),
			},
			want: `[{"name":"python"},{"name":"shell"}]`,
		},
		{
			name: "拒绝混合 PUSH 和 PULL 节点",
			pools: []domain.ExecutionPool{
				poolFixture(domain.ExecutionPoolKindExecutor, domain.ExecutionTransportGRPC, domain.ExecModePush, "", `[{"name":"shell"}]`),
				poolFixture(domain.ExecutionPoolKindExecutor, domain.ExecutionTransportGRPC, domain.ExecModePull, "", `[{"name":"shell"}]`),
			},
			wantErr: "不一致",
		},
		{
			name: "拒绝同池不同 Agent Topic",
			pools: []domain.ExecutionPool{
				poolFixture(domain.ExecutionPoolKindAgent, domain.ExecutionTransportMQ, domain.ExecModePush, "topic-a", `[{"name":"shell"}]`),
				poolFixture(domain.ExecutionPoolKindAgent, domain.ExecutionTransportMQ, domain.ExecModePush, "topic-b", `[{"name":"shell"}]`),
			},
			wantErr: "Topic 不一致",
		},
		{
			name: "拒绝同名 Handler 的不同定义",
			pools: []domain.ExecutionPool{
				poolFixture(domain.ExecutionPoolKindExecutor, domain.ExecutionTransportGRPC, domain.ExecModePush, "", `[{"name":"shell","desc":"A"}]`),
				poolFixture(domain.ExecutionPoolKindExecutor, domain.ExecutionTransportGRPC, domain.ExecModePush, "", `[{"name":"shell","desc":"B"}]`),
			},
			wantErr: "参数定义不一致",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			pool, err := aggregatePoolInstances(testCase.pools)
			if testCase.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
					t.Fatalf("aggregatePoolInstances() 错误 = %v, 期望包含 %q", err, testCase.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("aggregatePoolInstances() 返回意外错误: %v", err)
			}
			if pool.Metadata["supported_handlers"] != testCase.want {
				t.Fatalf("合并 Handler = %s, 期望 %s", pool.Metadata["supported_handlers"], testCase.want)
			}
			if _, exists := pool.Metadata["instance_id"]; exists {
				t.Fatal("资源池元数据不应保留单个实例 ID")
			}
		})
	}
}

func poolFixture(kind domain.ExecutionPoolKind, transport domain.ExecutionTransport,
	dispatchMode domain.ExecMode, topic, handlers string) domain.ExecutionPool {
	return domain.ExecutionPool{
		Name: "pool", Kind: kind, Transport: transport, DispatchMode: dispatchMode,
		IsolationLevel: domain.ExecutionPoolIsolationShared,
		Metadata: map[string]string{
			"topic": topic, "supported_handlers": handlers, "instance_id": "node-1",
		},
	}
}
