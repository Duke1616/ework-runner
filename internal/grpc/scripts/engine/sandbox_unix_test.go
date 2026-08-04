//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package engine

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCredentialProcessLauncherRunsWithRequestedIdentity(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("真实身份降权测试要求 root")
	}
	launcher, err := NewCredentialProcessLauncher(65534, 65534)
	require.NoError(t, err)
	command := exec.Command("/bin/sh", "-c", "id -u; id -g; id -G")
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	require.NoError(t, launcher.Start(command))
	require.NoError(t, command.Wait(), output.String())
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	require.Len(t, lines, 3)
	require.Equal(t, "65534", lines[0])
	require.Equal(t, "65534", lines[1])
	for _, group := range strings.Fields(lines[2]) {
		require.Equal(t, "65534", group)
	}
}
