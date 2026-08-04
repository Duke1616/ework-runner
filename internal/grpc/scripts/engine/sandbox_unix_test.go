//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package engine

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigureCommandSandboxRunsWithRequestedIdentity(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("真实身份降权测试要求 root")
	}
	command := exec.Command("/bin/sh", "-c", "id -u; id -g; id -G")
	require.NoError(t, configureCommandSandbox(command, Sandbox{Enabled: true, UID: 65534, GID: 65534}))
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	require.Len(t, lines, 3)
	require.Equal(t, "65534", lines[0])
	require.Equal(t, "65534", lines[1])
	for _, group := range strings.Fields(lines[2]) {
		require.Equal(t, "65534", group)
	}
}
