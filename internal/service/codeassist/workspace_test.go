package codeassist

import (
	"errors"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestNormalizeWorkspacePath(t *testing.T) {
	path, err := normalizeWorkspacePath(`roles\web\tasks\main.yml`)
	require.NoError(t, err)
	require.Equal(t, "roles/web/tasks/main.yml", path)

	for _, invalid := range []string{"", "/etc/passwd", "../secret", "roles/../../secret"} {
		_, err = normalizeWorkspacePath(invalid)
		require.Error(t, err, invalid)
	}
}

func TestWorkspaceReadReturnsRecoverableToolErrorForUnknownPath(t *testing.T) {
	service := &service{}
	_, err := service.readWorkspaceFiles(t.Context(), 3, preparedContext{},
		`{"paths":["missing.yml"]}`, &workspaceReadBudget{files: make(map[string]int)})

	var toolErr *workspaceToolError
	require.True(t, errors.As(err, &toolErr))
	require.Contains(t, toolErr.Error(), "does not exist")
}

func TestValidateCreateParentRejectsReadonlyWorkspace(t *testing.T) {
	workspace := map[string]domain.WorkspaceNode{
		"system": {
			RuntimePath: "system", Kind: domain.CodebookKindDirectory,
			Layer: domain.WorkspaceLayerSystem, Readonly: true,
		},
	}

	err := validateCreateParent("system/roles/demo/tasks/main.yml", workspace)

	require.EqualError(t, err, "AI change set cannot create inside readonly path: system")
}

func TestValidateCandidateSupportsYAML(t *testing.T) {
	require.Empty(t, validateCandidate(t.Context(), "yaml", "---\n- hosts: all\n"))
	diagnostics := validateCandidate(t.Context(), "yaml", "---\n- hosts: [\n")
	require.Len(t, diagnostics, 1)
	require.Equal(t, "SYNTAX_ERROR", diagnostics[0].Code)
}

func TestWorkspaceCredentialFilesAreNotReadableByAI(t *testing.T) {
	require.True(t, sensitiveWorkspacePath("inventory/.env.production"))
	require.True(t, sensitiveWorkspacePath("credentials/deploy.pem"))
	require.False(t, sensitiveWorkspacePath("inventory/hosts.yml"))
	require.True(t, containsSensitiveWorkspaceContent("ansible_password: secret"))
	require.False(t, containsSensitiveWorkspaceContent("etask_credential_ref: production"))
}
