package engine_test

import (
	"context"
	"testing"

	"github.com/Duke1616/etask/sdk/executor"
	"github.com/Duke1616/etask/sdk/executor/engine"
	"github.com/stretchr/testify/require"
)

func TestPublicEngineContract(t *testing.T) {
	handlers := engine.NewHandlerRegistry()
	handler := publicHandler{}
	require.NoError(t, handlers.Register(handler))
	require.ErrorContains(t, handlers.Register(handler), "名称重复")

	ctx := context.WithValue(t.Context(), contextKey{}, "caller")
	result, err := engine.New(handlers, nil).Execute(ctx, engine.Command{
		Task: executor.TaskInfo{ExecutionID: 1, Handler: handler.Name()},
	})
	require.NoError(t, err)
	require.JSONEq(t, `{"context":"caller"}`, result.Value)
}

type contextKey struct{}

type publicHandler struct{}

func (publicHandler) Name() string                   { return "public" }
func (publicHandler) Desc() string                   { return "public contract test" }
func (publicHandler) Metadata() []executor.Parameter { return nil }
func (publicHandler) Run(ctx *executor.Context) error {
	ctx.SetResult("context", ctx.Context().Value(contextKey{}))
	return nil
}

var _ executor.TaskHandler = publicHandler{}
