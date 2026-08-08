package inventory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStaticResolverYAML(t *testing.T) {
	inventory := filepath.Join(t.TempDir(), "hosts.yml")
	require.NoError(t, os.WriteFile(inventory, []byte(`all:
  vars:
    etask_credential_ref: default-key
  children:
    linux:
      vars:
        etask_credential_ref: linux-key
      hosts:
        linux-1: {}
        override-1:
          etask_credential_ref: host-password
    inherited:
      hosts:
        inherited-1: {}
`), 0o600))

	plan, err := (StaticResolver{}).Resolve(inventory)
	require.NoError(t, err)
	require.Equal(t, []string{"inherited-1", "linux-1", "override-1"}, plan.Hosts)
	require.Equal(t, map[string]string{
		"inherited-1": "default-key",
		"linux-1":     "linux-key",
		"override-1":  "host-password",
	}, plan.References)
}

func TestStaticResolverINI(t *testing.T) {
	inventory := filepath.Join(t.TempDir(), "hosts.ini")
	require.NoError(t, os.WriteFile(inventory, []byte(`[all:vars]
etask_credential_ref=default-key

[linux]
linux-1
override-1 etask_credential_ref=host-password

[linux:vars]
etask_credential_ref=linux-key

[plain]
plain-1
`), 0o600))

	plan, err := (StaticResolver{}).Resolve(inventory)
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"linux-1":    "linux-key",
		"override-1": "host-password",
		"plain-1":    "default-key",
	}, plan.References)
}

func TestStaticResolverRejectsSamePriorityConflict(t *testing.T) {
	inventory := filepath.Join(t.TempDir(), "hosts.yml")
	require.NoError(t, os.WriteFile(inventory, []byte(`all:
  children:
    group_a:
      vars:
        etask_credential_ref: key-a
      hosts:
        shared: {}
    group_b:
      vars:
        etask_credential_ref: key-b
      hosts:
        shared: {}
`), 0o600))

	_, err := (StaticResolver{}).Resolve(inventory)
	require.ErrorContains(t, err, "多个同优先级凭据")
}
