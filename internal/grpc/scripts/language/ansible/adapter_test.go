package ansible

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Duke1616/etask/internal/grpc/scripts/engine"
	"github.com/Duke1616/etask/internal/grpc/scripts/language/ansible/connection"
	"github.com/Duke1616/etask/sdk/executor"
	"github.com/stretchr/testify/require"
)

func TestAdapterPrepare(t *testing.T) {
	workspace := newWorkspace(t)
	adapter := New("/usr/local/bin/ansible-playbook")
	prepared, err := adapter.Prepare(t.Context(), workspace, engine.Input{
		Args: `{"environment":"production","replicas":3,"deployment":{"strategy":"rolling"}}`,
		Variables: `[{"key":"ansible_user","value":"deploy","secret":false},` +
			`{"key":"environment","value":"staging","secret":false},` +
			`{"key":"replicas","value":"2","secret":false}]`,
		Params: map[string]string{
			"inventory": "inventory/staging.yml", "limit": "web:&staging", "tags": "deploy",
			"check": "true", "forks": "10", "verbosity": "2",
			"extra_args": `--start-at-task "Deploy application"`,
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{
		"/usr/local/bin/ansible-playbook",
		"--inventory", filepath.Join(workspace.ProgramRoot(), "inventory", "staging.yml"),
		"--limit", "web:&staging", "--tags", "deploy", "--check", "--forks", "10", "-vv",
		"--start-at-task", "Deploy application",
		"--extra-vars",
		"@" + filepath.Join(workspace.root, "ansible-extra-vars.json"), workspace.entryPoint,
	}, prepared.Command.Args)
	require.Contains(t, prepared.Environment, "ANSIBLE_HOME="+filepath.Join(workspace.root, ".ansible"))
	require.Contains(t, prepared.Environment, "ANSIBLE_LOCAL_TEMP="+filepath.Join(workspace.root, ".ansible-tmp"))
	require.Contains(t, prepared.Environment, "ANSIBLE_RETRY_FILES_ENABLED=False")
	require.NotContains(t, prepared.Environment, "ANSIBLE_HOST_KEY_CHECKING=True")
	require.NotContains(t, prepared.Environment, "ETASK_ARGS_FILE="+filepath.Join(workspace.root, "args.json"))

	content, err := os.ReadFile(filepath.Join(workspace.root, "ansible-extra-vars.json"))
	require.NoError(t, err)
	info, err := os.Stat(filepath.Join(workspace.root, "ansible-extra-vars.json"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	var extraVars map[string]any
	require.NoError(t, json.Unmarshal(content, &extraVars))
	require.Equal(t, "deploy", extraVars["ansible_user"])
	require.Equal(t, "staging", extraVars["environment"])
	require.Equal(t, "2", extraVars["replicas"])
	require.Equal(t, map[string]any{
		"environment": "production", "replicas": float64(3),
		"deployment": map[string]any{"strategy": "rolling"},
	}, extraVars["args"])
}

func TestAdapterPrepareRejectsInvalidExecutionArgs(t *testing.T) {
	workspace := newWorkspace(t)
	_, err := New("/usr/local/bin/ansible-playbook").Prepare(t.Context(), workspace, engine.Input{
		Args: `{`,
	})
	require.ErrorContains(t, err, "解析 Ansible 执行参数失败")
}

func TestAdapterPrepareInjectsLocalSSHCredential(t *testing.T) {
	workspace := newWorkspace(t)
	credentialRoot := t.TempDir()
	privateKey := writeTestPrivateKey(t, credentialRoot, "production-key", 0o600)
	knownHosts := filepath.Join(t.TempDir(), "known_hosts")
	require.NoError(t, os.WriteFile(knownHosts, []byte("10.0.0.10 ssh-ed25519 AAAAC3NzaTest\n"), 0o644))
	credentialProvider, err := connection.NewLocalCredentialProvider(credentialRoot, map[string]connection.CredentialConfig{
		"production-linux": {Username: "deploy", PrivateKeyFile: "production-key"},
	})
	require.NoError(t, err)
	hostKeyProvider, err := connection.NewFileHostKeyProvider(knownHosts)
	require.NoError(t, err)
	connections := connection.NewSSHPreparer(credentialProvider, hostKeyProvider)
	adapter := New("/usr/local/bin/ansible-playbook", WithSSHConnectionPreparer(connections))

	prepared, err := adapter.Prepare(t.Context(), workspace, engine.Input{
		Variables: `[{"key":"environment","value":"production"}]`,
		Params: map[string]string{
			"credential_ref": "production-linux",
		},
	})
	require.NoError(t, err)
	require.NotContains(t, strings.Join(prepared.Command.Args, " "), string(privateKey))
	require.Contains(t, prepared.Environment, "ANSIBLE_HOST_KEY_CHECKING=True")

	privateKeyFiles, err := filepath.Glob(filepath.Join(workspace.root, "ansible-ssh-private-key-*"))
	require.NoError(t, err)
	require.Len(t, privateKeyFiles, 1)
	privateKeyFile := privateKeyFiles[0]
	content, err := os.ReadFile(privateKeyFile)
	require.NoError(t, err)
	require.Equal(t, privateKey, content)
	info, err := os.Stat(privateKeyFile)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	extraVarsContent, err := os.ReadFile(filepath.Join(workspace.root, "ansible-extra-vars.json"))
	require.NoError(t, err)
	var extraVars map[string]any
	require.NoError(t, json.Unmarshal(extraVarsContent, &extraVars))
	require.Equal(t, "production", extraVars["environment"])
	require.Equal(t, "deploy", extraVars["ansible_user"])
	require.Equal(t, privateKeyFile, extraVars["ansible_ssh_private_key_file"])
	require.Contains(t, extraVars["ansible_ssh_common_args"].(string), "StrictHostKeyChecking=yes")
	require.Contains(t, extraVars["ansible_ssh_common_args"].(string), filepath.Join(workspace.root, "ansible-known-hosts"))
}

func TestAdapterPrepareBuildsInventoryConnectionPlan(t *testing.T) {
	workspace := newWorkspace(t)
	inventoryFile := filepath.Join(workspace.ProgramRoot(), "inventory", "staging.yml")
	require.NoError(t, os.Chmod(inventoryFile, 0o600))
	require.NoError(t, os.WriteFile(inventoryFile, []byte(`all:
  children:
    modern:
      vars:
        etask_credential_ref: production-key
      hosts:
        modern-1: {}
    legacy:
      vars:
        etask_credential_ref: legacy-password
      hosts:
        legacy-1: {}
`), 0o440))
	credentialRoot := t.TempDir()
	writeTestPrivateKey(t, credentialRoot, "production-key", 0o600)
	require.NoError(t, os.WriteFile(filepath.Join(credentialRoot, "legacy-password"), []byte("legacy-secret\n"), 0o600))
	knownHosts := filepath.Join(t.TempDir(), "known_hosts")
	require.NoError(t, os.WriteFile(knownHosts, []byte("host ssh-ed25519 AAAATest\n"), 0o644))
	provider, err := connection.NewLocalCredentialProvider(credentialRoot, map[string]connection.CredentialConfig{
		"production-key":  {Type: "private_key", Username: "deploy", PrivateKeyFile: "production-key"},
		"legacy-password": {Type: "password", Username: "root", PasswordFile: "legacy-password"},
	})
	require.NoError(t, err)
	hostKeys, err := connection.NewFileHostKeyProvider(knownHosts)
	require.NoError(t, err)
	adapter := New("ansible-playbook", WithSSHConnectionPreparer(connection.NewSSHPreparer(
		provider, hostKeys, connection.WithSSHPassBinary("/bin/sh"),
	)))

	prepared, err := adapter.Prepare(t.Context(), workspace, engine.Input{Params: map[string]string{
		"inventory": "inventory/staging.yml",
	}})
	require.NoError(t, err)
	require.Equal(t, []string{"legacy-secret"}, prepared.SecretMasks)
	extraVarsContent, err := os.ReadFile(filepath.Join(workspace.root, "ansible-extra-vars.json"))
	require.NoError(t, err)
	var extraVars map[string]any
	require.NoError(t, json.Unmarshal(extraVarsContent, &extraVars))
	hosts := extraVars[connection.MapVariable].(map[string]any)
	modern := hosts["modern-1"].(map[string]any)
	legacy := hosts["legacy-1"].(map[string]any)
	require.Equal(t, "deploy", modern["ansible_user"])
	require.Empty(t, modern["ansible_password"])
	require.NotEmpty(t, modern["ansible_ssh_private_key_file"])
	require.Equal(t, "root", legacy["ansible_user"])
	require.Equal(t, "legacy-secret", legacy["ansible_password"])
	require.Empty(t, legacy["ansible_ssh_private_key_file"])
	require.Contains(t, extraVars["ansible_user"], connection.MapVariable)
}

func TestInventoryConnectionPlanRendersPerHostInAnsible(t *testing.T) {
	binary, err := exec.LookPath("ansible-playbook")
	if err != nil {
		t.Skip("当前环境未安装 ansible-playbook")
	}
	workspace := newWorkspace(t)
	inventoryFile := filepath.Join(workspace.ProgramRoot(), "inventory", "staging.yml")
	require.NoError(t, os.Chmod(inventoryFile, 0o600))
	require.NoError(t, os.WriteFile(inventoryFile, []byte(`all:
  children:
    modern:
      vars:
        etask_credential_ref: production-key
      hosts:
        modern-1:
          ansible_connection: local
    legacy:
      vars:
        etask_credential_ref: legacy-password
      hosts:
        legacy-1:
          ansible_connection: local
`), 0o440))
	require.NoError(t, os.Chmod(workspace.entryPoint, 0o600))
	require.NoError(t, os.WriteFile(workspace.entryPoint, []byte(`---
- hosts: all
  gather_facts: false
  tasks:
    - name: Verify resolved connection variables
      ansible.builtin.assert:
        that:
          - ansible_user == ('deploy' if inventory_hostname == 'modern-1' else 'root')
          - ansible_password == ('' if inventory_hostname == 'modern-1' else 'legacy-secret')
          - (ansible_ssh_private_key_file | length > 0) if inventory_hostname == 'modern-1' else (ansible_ssh_private_key_file == '')
`), 0o440))
	credentialRoot := t.TempDir()
	writeTestPrivateKey(t, credentialRoot, "production-key", 0o600)
	require.NoError(t, os.WriteFile(filepath.Join(credentialRoot, "legacy-password"), []byte("legacy-secret\n"), 0o600))
	knownHosts := filepath.Join(t.TempDir(), "known_hosts")
	require.NoError(t, os.WriteFile(knownHosts, []byte("host ssh-ed25519 AAAATest\n"), 0o644))
	provider, err := connection.NewLocalCredentialProvider(credentialRoot, map[string]connection.CredentialConfig{
		"production-key":  {Username: "deploy", PrivateKeyFile: "production-key"},
		"legacy-password": {Type: "password", Username: "root", PasswordFile: "legacy-password"},
	})
	require.NoError(t, err)
	hostKeys, err := connection.NewFileHostKeyProvider(knownHosts)
	require.NoError(t, err)
	adapter := New(binary, WithSSHConnectionPreparer(connection.NewSSHPreparer(provider, hostKeys)))
	prepared, err := adapter.Prepare(t.Context(), workspace, engine.Input{Params: map[string]string{
		"inventory": "inventory/staging.yml",
	}})
	require.NoError(t, err)
	prepared.Command.Dir = workspace.ProgramRoot()
	prepared.Command.Env = append(os.Environ(), prepared.Environment...)
	output, err := prepared.Command.CombinedOutput()
	require.NoError(t, err, string(output))
}

func TestBuildExtraVarsRejectsInvalidInput(t *testing.T) {
	testCases := []struct {
		name      string
		vars      string
		wantError string
	}{
		{name: "格式不是数组", vars: `{}`, wantError: "必须是变量数组"},
		{name: "变量名称非法", vars: `[{"key":"bad-key","value":"x"}]`, wantError: "名称非法"},
		{name: "连接密码必须使用凭据", vars: `[{"key":"ansible_password","value":"secret","secret":true}]`, wantError: "credential_ref"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := buildExtraVars(testCase.vars)
			require.ErrorContains(t, err, testCase.wantError)
		})
	}
}

func TestAdapterProgramKinds(t *testing.T) {
	require.Equal(t, []executor.ProgramKind{executor.ProgramKindProject}, New("").ProgramKinds())
}

func TestAdapterMetadataDeclaresCommonOptions(t *testing.T) {
	components := make(map[string]string)
	var vars executor.Parameter
	for _, parameter := range New("").Metadata() {
		if parameter.Key == "vars" {
			vars = parameter
			binding, ok := parameter.Bindings["manual"].(*executor.BindingOption)
			require.True(t, ok)
			components[parameter.Key] = binding.Component
		}
		binding, ok := parameter.Bindings["static"].(*executor.BindingOption)
		if ok {
			components[parameter.Key] = binding.Component
		}
	}

	require.Equal(t, "project-file-picker", components["inventory"])
	require.Equal(t, "select-input", components["credential_ref"])
	require.Equal(t, "input", components["limit"])
	require.Equal(t, "input", components["tags"])
	require.Equal(t, "boolean-switch", components["check"])
	require.Equal(t, "number-input", components["forks"])
	require.Equal(t, "select-input", components["verbosity"])
	require.Equal(t, "input", components["extra_args"])
	require.NotContains(t, components, "options")
	require.Equal(t, "code-editor", components["args"])
	require.Equal(t, "kv-input", components["vars"])
	require.Equal(t, executor.ParameterRoleVariables, vars.Role)
	for _, parameter := range New("").Metadata() {
		if parameter.Key == "args" {
			require.Equal(t, executor.ParameterRoleArgs, parameter.Role)
		}
	}
	require.Contains(t, vars.Bindings, "manual")
	require.Contains(t, vars.Bindings, "runner")
}

func TestAdapterMetadataDeclaresRuntimeOverridableOptions(t *testing.T) {
	got := make(map[string]bool)
	for _, parameter := range New("").Metadata() {
		got[parameter.Key] = parameter.RuntimeOverridable
	}

	for _, key := range []string{"args", "limit", "skip_tags", "tags"} {
		if !got[key] {
			t.Fatalf("parameter %s should allow runtime override", key)
		}
	}
	for _, key := range []string{"inventory", "credential_ref", "check", "vars", "extra_args"} {
		if got[key] {
			t.Fatalf("parameter %s should not allow runtime override", key)
		}
	}
}

func TestAdapterMetadataListsLocalCredentials(t *testing.T) {
	credentialProvider, err := connection.NewLocalCredentialProvider(t.TempDir(), map[string]connection.CredentialConfig{
		"production-b": {Username: "deploy", PrivateKeyFile: "b-key"},
		"production-a": {Username: "deploy", PrivateKeyFile: "a-key"},
	})
	require.NoError(t, err)
	hostKeyProvider, err := connection.NewFileHostKeyProvider(filepath.Join(t.TempDir(), "known_hosts"))
	require.NoError(t, err)
	connections := connection.NewSSHPreparer(credentialProvider, hostKeyProvider)
	for _, parameter := range New("", WithSSHConnectionPreparer(connections)).Metadata() {
		if parameter.Key != "credential_ref" {
			continue
		}
		binding, ok := parameter.Bindings["static"].(*executor.BindingOption)
		require.True(t, ok)
		require.JSONEq(t, `[
			{"label":"production-a","value":"production-a"},
			{"label":"production-b","value":"production-b"}
		]`, binding.Config["options"])
		return
	}
	t.Fatal("credential_ref metadata not found")
}

func TestAdapterValidate(t *testing.T) {
	require.NoError(t, New(filepath.Join(t.TempDir(), "missing")).Validate())
}

type workspaceStub struct {
	root       string
	entryPoint string
}

func newWorkspace(t *testing.T) *workspaceStub {
	root := t.TempDir()
	entryPoint := filepath.Join(root, "project", "playbooks", "deploy.yml")
	inventory := filepath.Join(root, "project", "inventory", "staging.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(entryPoint), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Dir(inventory), 0o750))
	require.NoError(t, os.WriteFile(entryPoint, []byte("---\n"), 0o440))
	require.NoError(t, os.WriteFile(inventory, []byte("all:\n"), 0o440))
	return &workspaceStub{root: root, entryPoint: entryPoint}
}

func (w *workspaceStub) ProgramRoot() string   { return filepath.Dir(filepath.Dir(w.entryPoint)) }
func (w *workspaceStub) EntryPoint() string    { return w.entryPoint }
func (w *workspaceStub) Environment() []string { return nil }
func (w *workspaceStub) Close() error          { return nil }
func (w *workspaceStub) WriteFile(name string, content []byte, mode os.FileMode) (string, error) {
	path := filepath.Join(w.root, name)
	return path, os.WriteFile(path, content, mode)
}

var _ engine.Workspace = (*workspaceStub)(nil)
