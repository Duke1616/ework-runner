package runtimefs

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewIsolatedWorkspaceAccessRejectsRootIdentity(t *testing.T) {
	_, err := NewIsolatedWorkspaceAccess(0, 1000)
	require.ErrorContains(t, err, "非 root 身份")
	_, err = NewIsolatedWorkspaceAccess(1000, 0)
	require.ErrorContains(t, err, "非 root 身份")
}
