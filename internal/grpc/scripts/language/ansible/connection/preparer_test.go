package connection

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Duke1616/etask/internal/grpc/scripts/engine"
	"github.com/stretchr/testify/require"
)

func TestSSHPreparerRequiresDefaultForUnboundInventoryHosts(t *testing.T) {
	inventoryFile := filepath.Join(t.TempDir(), "hosts.yml")
	require.NoError(t, os.WriteFile(inventoryFile, []byte(`all:
  hosts:
    managed:
      etask_credential_ref: managed-key
    unbound: {}
`), 0o600))
	root := t.TempDir()
	writeTestPrivateKey(t, root, "managed-key", 0o600)
	provider, err := NewLocalCredentialProvider(root, map[string]CredentialConfig{
		"managed-key": {Username: "deploy", PrivateKeyFile: "managed-key"},
	})
	require.NoError(t, err)
	hostsFile := filepath.Join(t.TempDir(), "known_hosts")
	require.NoError(t, os.WriteFile(hostsFile, []byte("host ssh-ed25519 AAAATest\n"), 0o644))
	hostKeys, err := NewFileHostKeyProvider(hostsFile)
	require.NoError(t, err)
	preparer := NewSSHPreparer(provider, hostKeys)
	_, err = preparer.Prepare(newConnectionWorkspace(t), Request{InventoryFile: inventoryFile})
	require.ErrorContains(t, err, "unbound")
}

type connectionWorkspaceStub struct {
	root string
}

func newConnectionWorkspace(t *testing.T) *connectionWorkspaceStub {
	t.Helper()
	return &connectionWorkspaceStub{root: t.TempDir()}
}

func (w *connectionWorkspaceStub) ProgramRoot() string   { return w.root }
func (w *connectionWorkspaceStub) EntryPoint() string    { return "" }
func (w *connectionWorkspaceStub) Environment() []string { return nil }
func (w *connectionWorkspaceStub) Close() error          { return nil }
func (w *connectionWorkspaceStub) WriteFile(name string, content []byte, mode os.FileMode) (string, error) {
	file := filepath.Join(w.root, name)
	return file, os.WriteFile(file, content, mode)
}

var _ engine.Workspace = (*connectionWorkspaceStub)(nil)
