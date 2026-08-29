package binding

import (
	"context"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestRegistryResolve(t *testing.T) {
	testCases := []struct {
		name       string
		before     func(registry *Registry)
		params     map[string]string
		metadata   map[string]string
		wantValues map[string]string
	}{
		{
			name: "按参数名稳定解析",
			before: func(registry *Registry) {
				registry.Register("resource", ResolverFunc(func(_ context.Context, req ResolveRequest) (ResolveResult, error) {
					return ResolveResult{Parameters: map[string]string{req.ParamKey: "resolved-" + req.Value}}, nil
				}))
			},
			params: map[string]string{"z": "2", "a": "1"}, metadata: map[string]string{"z": "resource", "a": "resource"},
			wantValues: map[string]string{"a": "resolved-1", "z": "resolved-2"},
		},
		{name: "未注册绑定保持为空", params: map[string]string{"a": "1"}, metadata: map[string]string{"a": "missing"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			registry := NewRegistry()
			if tc.before != nil {
				tc.before(registry)
			}
			result, err := registry.Resolve(t.Context(), "shell", tc.params, tc.metadata)
			require.NoError(t, err)
			require.Equal(t, tc.wantValues, result.Parameters)
		})
	}
}

func TestRegistryResolveTypedSeparatesVariables(t *testing.T) {
	registry := NewRegistry().Register("runner", typedTestResolver{})
	result, err := registry.Resolve(t.Context(), "shell",
		map[string]string{"variables": "runner-1"}, map[string]string{"variables": "runner"})
	require.NoError(t, err)
	require.Empty(t, result.Parameters)
	require.Equal(t, []domain.RunnerVariable{{Key: "TOKEN", Value: "secret", Secret: true}}, result.Variables)
}

func TestRegistryTrimsBindingNames(t *testing.T) {
	registry := NewRegistry().Register(" runner ", typedTestResolver{})
	result, err := registry.Resolve(t.Context(), "shell",
		map[string]string{"variables": "1"}, map[string]string{"variables": " runner "})
	require.NoError(t, err)
	require.Len(t, result.Variables, 1)
}

type typedTestResolver struct{}

func (typedTestResolver) Resolve(context.Context, ResolveRequest) (ResolveResult, error) {
	return ResolveResult{Variables: []domain.RunnerVariable{{Key: "TOKEN", Value: "secret", Secret: true}}}, nil
}
