package runtimefs

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewIsolatedWorkspaceAccess(t *testing.T) {
	testCases := []struct {
		name      string
		uid       uint32
		gid       uint32
		wantErr   string
		assertEnv func(t *testing.T, access WorkspaceAccess)
	}{
		{
			name:    "拒绝 UID 为 0",
			uid:     0,
			gid:     1000,
			wantErr: "非 root 身份",
		},
		{
			name:    "拒绝 GID 为 0",
			uid:     1000,
			gid:     0,
			wantErr: "非 root 身份",
		},
		{
			name: "非 root 身份正常构造并注入标准环境",
			uid:  1001,
			gid:  1002,
			assertEnv: func(t *testing.T, access WorkspaceAccess) {
				env := access.Environment("/workspace/root")
				require.Contains(t, env, "HOME=/workspace/root")
				require.Contains(t, env, "TMPDIR="+filepath.Join("/workspace/root", "tmp"))
				require.Contains(t, env, "PYTHONDONTWRITEBYTECODE=1")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			access, err := NewIsolatedWorkspaceAccess(tc.uid, tc.gid)
			if tc.wantErr != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, access)
			if tc.assertEnv != nil {
				tc.assertEnv(t, access)
			}
		})
	}
}

func TestHostWorkspaceAccess(t *testing.T) {
	host := NewHostWorkspaceAccess()
	require.NoError(t, host.PrepareRoot("/tmp"))
	require.NoError(t, host.PrepareWorkspace("/tmp"))
	require.NoError(t, host.Own("/tmp"))
	require.Nil(t, host.Environment("/tmp"))
}
