package input

import (
	"context"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/pkg/variable"
	taskbinding "github.com/Duke1616/etask/internal/service/task/binding"
	"github.com/stretchr/testify/require"
)

func TestExecutionInputAssemblerKeepsParameterAndVariablePriorityExplicit(t *testing.T) {
	resolvers := taskbinding.NewRegistry().Register("binding", taskbinding.ResolverFunc(
		func(_ context.Context, req taskbinding.ResolveRequest) (taskbinding.ResolveResult, error) {
			return taskbinding.ResolveResult{
				Parameters: map[string]string{req.ParamKey: "binding"},
				Variables: []variable.Item{
					{Key: "TOKEN", Value: "binding", Secret: true},
				},
			}, nil
		},
	))
	assembler := NewExecutionInputAssembler(nil, resolvers)

	result, err := assembler.Assemble(context.Background(), domain.Task{
		GrpcConfig: &domain.GrpcConfig{
			HandlerName: "shell",
			Params:      map[string]string{"region": "task", "binding_param": "raw"},
			Variables:   []variable.Item{{Key: "TOKEN", Value: "task", Secret: false}},
		},
		Metadata:              map[string]string{"region": "binding"},
		PendingParamOverrides: map[string]string{"region": "runtime"},
	})

	require.NoError(t, err)
	require.Equal(t, "runtime", result.Task.GrpcConfig.Params["region"])
	require.Equal(t, "raw", result.Task.GrpcConfig.Params["binding_param"])
	require.Equal(t, []variable.Item{{Key: "TOKEN", Value: "binding", Secret: true}}, result.Variables.Items)
}
