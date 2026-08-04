package binding

import (
	"context"
	"testing"

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
				registry.Register("resource", ResolverFunc(func(_ context.Context, req ResolveRequest) (string, error) {
					return "resolved-" + req.Value, nil
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
			require.Equal(t, tc.wantValues, result)
		})
	}
}
