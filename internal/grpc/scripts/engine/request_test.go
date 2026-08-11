package engine

import (
	"testing"

	"github.com/Duke1616/etask/sdk/executor"
	"github.com/stretchr/testify/require"
)

func TestResolveRequestUsesIndependentVariablesForSemanticParameter(t *testing.T) {
	task := executor.NewContext(executor.ContextOptions{
		Params: map[string]string{"inventory": "inventory.yml"},
		Variables: &executor.VariableSet{Items: []executor.Variable{
			{Key: "environment", Value: "production"},
			{Key: "token", Value: "secret", Secret: true},
		}},
	})
	task.SetProgram(&executor.Program{Kind: executor.ProgramKindProject, Project: &executor.ProjectProgram{}})

	request, err := resolveRequest(task, "ansible", []executor.Parameter{
		{Key: "vars", Role: executor.ParameterRoleVariables},
		{Key: "inventory"},
	}, Config{MaxArgsSize: 1024, MaxVariablesSize: 1024})

	require.NoError(t, err)
	require.JSONEq(t, `[
		{"key":"environment","value":"production","secret":false},
		{"key":"token","value":"secret","secret":true}
	]`, request.input.Variables)
	require.Equal(t, "inventory.yml", request.input.Params["inventory"])
	require.NotContains(t, request.input.Params, "vars")
}

func TestResolveRequestKeepsExplicitVariableParameterCompatible(t *testing.T) {
	task := executor.NewContext(executor.ContextOptions{
		Params:    map[string]string{"vars": `[{"key":"source","value":"manual"}]`},
		Variables: &executor.VariableSet{Items: []executor.Variable{{Key: "source", Value: "runner"}}},
	})
	task.SetProgram(&executor.Program{Kind: executor.ProgramKindProject, Project: &executor.ProjectProgram{}})

	request, err := resolveRequest(task, "ansible", []executor.Parameter{
		{Key: "vars", Role: executor.ParameterRoleVariables},
	}, Config{MaxArgsSize: 1024, MaxVariablesSize: 1024})

	require.NoError(t, err)
	require.JSONEq(t, `[{"key":"source","value":"manual"}]`, request.input.Variables)
}

func TestResolveRequestPreservesExplicitEmptyVariableSet(t *testing.T) {
	task := executor.NewContext(executor.ContextOptions{Variables: &executor.VariableSet{Items: []executor.Variable{}}})
	task.SetProgram(&executor.Program{Kind: executor.ProgramKindProject, Project: &executor.ProjectProgram{}})

	request, err := resolveRequest(task, "ansible", []executor.Parameter{
		{Key: "vars", Role: executor.ParameterRoleVariables},
	}, Config{MaxArgsSize: 1024, MaxVariablesSize: 1024})

	require.NoError(t, err)
	require.Equal(t, "[]", request.input.Variables)
}

func TestResolveRequestUsesArgsSemanticRoleWithoutKeepingGenericParam(t *testing.T) {
	task := executor.NewContext(executor.ContextOptions{Params: map[string]string{"payload": `{"id":1}`}})
	task.SetProgram(&executor.Program{Kind: executor.ProgramKindProject, Project: &executor.ProjectProgram{}})

	request, err := resolveRequest(task, "shell", []executor.Parameter{
		{Key: "payload", Role: executor.ParameterRoleArgs},
	}, Config{MaxArgsSize: 1024, MaxVariablesSize: 1024})

	require.NoError(t, err)
	require.JSONEq(t, `{"id":1}`, request.input.Args)
	require.NotContains(t, request.input.Params, "payload")
}
