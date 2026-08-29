package source

import (
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/pkg/grpc/registry"
)

func TestNormalizeExecutorMode(t *testing.T) {
	testCases := []struct {
		name string
		mode any
		want domain.ExecMode
	}{
		{name: "empty", mode: "", want: domain.ExecModePush},
		{name: "missing", mode: nil, want: domain.ExecModePush},
		{name: "push uppercase", mode: "PUSH", want: domain.ExecModePush},
		{name: "push lowercase", mode: "push", want: domain.ExecModePush},
		{name: "pull uppercase", mode: "PULL", want: domain.ExecModePull},
		{name: "pull lowercase", mode: "pull", want: domain.ExecModePull},
		{name: "invalid", mode: "grpc", want: domain.ExecModePush},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			metadata := map[string]any{}
			if tc.mode != nil {
				metadata["mode"] = tc.mode
			}
			metadata["role"] = "executor"
			got := normalizeExecutorMode(registry.ServiceInstance{Metadata: metadata})
			if got != tc.want {
				t.Fatalf("normalizeExecutorMode() = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestBuildExecutorPoolNormalizesMode(t *testing.T) {
	pool, ok := buildExecutorPool(registry.ServiceInstance{
		Name: "execute",
		Metadata: map[string]any{
			"role": "executor",
			"mode": "pull",
		},
	})
	if !ok {
		t.Fatal("buildExecutorPool() returned ok=false")
	}
	if pool.Transport != domain.ExecutionTransportGRPC || pool.DispatchMode != domain.ExecModePull {
		t.Fatalf("pool route = %s/%s, want GRPC/PULL", pool.Transport, pool.DispatchMode)
	}
}

func TestBuildExecutorPoolReadsIsolationLevelFromRegistration(t *testing.T) {
	pool, ok := buildExecutorPool(registry.ServiceInstance{
		Name: "dedicated-executor",
		Metadata: map[string]any{
			"role":            "executor",
			"isolation_level": "dedicated",
		},
	})
	if !ok {
		t.Fatal("buildExecutorPool() returned ok=false")
	}
	if pool.IsolationLevel != domain.ExecutionPoolIsolationDedicated {
		t.Fatalf("IsolationLevel = %s, want %s", pool.IsolationLevel, domain.ExecutionPoolIsolationDedicated)
	}
}

func TestBuildExecutorPoolRejectsNonExecutor(t *testing.T) {
	_, ok := buildExecutorPool(registry.ServiceInstance{
		Name: "scheduler",
		Metadata: map[string]any{
			"role": "scheduler",
		},
	})
	if ok {
		t.Fatal("buildExecutorPool() returned ok=true for non-executor instance")
	}
}
