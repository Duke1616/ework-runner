package task

// Registry 测试覆盖排序、快照和并发访问。

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandlerRegistry(t *testing.T) {
	testCases := []struct {
		name       string
		before     func(registry *HandlerRegistry)
		after      func(t *testing.T, registry *HandlerRegistry)
		assertions func(t *testing.T, registry *HandlerRegistry)
	}{
		{
			name: "注册结果按名称稳定排序",
			before: func(registry *HandlerRegistry) {
				_ = registry.Register(handlerStub{name: "python"}, handlerStub{name: "shell"})
			},
			assertions: func(t *testing.T, registry *HandlerRegistry) {
				require.Equal(t, []string{"python", "shell"}, registry.Names())
				require.Equal(t, "python", registry.ListMetas()[0].Name)
			},
		},
		{
			name:   "快照修改不影响注册中心",
			before: func(registry *HandlerRegistry) { _ = registry.Register(handlerStub{name: "shell"}) },
			assertions: func(t *testing.T, registry *HandlerRegistry) {
				snapshot := registry.Snapshot()
				delete(snapshot, "shell")
				_, exists := registry.Get("shell")
				require.True(t, exists)
			},
		},
		{
			name: "允许并发读写",
			before: func(registry *HandlerRegistry) {
				var wait sync.WaitGroup
				for i := 0; i < 20; i++ {
					wait.Add(2)
					go func() { defer wait.Done(); _ = registry.Register(handlerStub{name: "shell"}) }()
					go func() { defer wait.Done(); _ = registry.Names() }()
				}
				wait.Wait()
			},
			assertions: func(t *testing.T, registry *HandlerRegistry) {
				require.Equal(t, []string{"shell"}, registry.Names())
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			registry := NewHandlerRegistry()
			if tc.before != nil {
				tc.before(registry)
			}
			if tc.after != nil {
				defer tc.after(t, registry)
			}
			tc.assertions(t, registry)
		})
	}
}

func TestHandlerRegistryRejectsDuplicateAtomically(t *testing.T) {
	registry := NewHandlerRegistry()
	err := registry.Register(handlerStub{name: "shell"}, handlerStub{name: "shell"})
	require.ErrorContains(t, err, "名称重复")
	require.Empty(t, registry.Names())

	require.NoError(t, registry.Register(handlerStub{name: "python"}))
	err = registry.Register(handlerStub{name: "shell"}, handlerStub{name: "python"})
	require.ErrorContains(t, err, "名称重复")
	require.Equal(t, []string{"python"}, registry.Names())
}

type handlerStub struct{ name string }

func (h handlerStub) Name() string          { return h.name }
func (h handlerStub) Desc() string          { return h.name }
func (h handlerStub) Metadata() []Parameter { return nil }
func (h handlerStub) Run(*Context) error    { return nil }

var _ TaskHandler = handlerStub{}
