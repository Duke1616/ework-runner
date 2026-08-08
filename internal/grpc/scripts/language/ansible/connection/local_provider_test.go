package connection

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalCredentialProviderResolvesPrivateKey(t *testing.T) {
	root := t.TempDir()
	privateKey := writeTestPrivateKey(t, root, "production-key", 0o600)
	knownHosts := filepath.Join(t.TempDir(), "known_hosts")
	require.NoError(t, os.WriteFile(knownHosts, []byte("server ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITest\n"), 0o644))

	provider, err := NewLocalCredentialProvider(root, map[string]CredentialConfig{
		"production-b": {Username: "deploy", PrivateKeyFile: "production-key"},
		"production-a": {Username: "deploy", PrivateKeyFile: "production-key"},
	})
	require.NoError(t, err)
	hostKeys, err := NewFileHostKeyProvider(knownHosts)
	require.NoError(t, err)
	require.NoError(t, NewSSHPreparer(provider, hostKeys).Validate())
	require.Equal(t, []string{"production-a", "production-b"}, provider.References())

	credential, err := provider.Resolve("production-a")
	require.NoError(t, err)
	require.Equal(t, "deploy", credential.Username)
	authentication, ok := credential.Authentication.(*PrivateKeyAuthentication)
	require.True(t, ok)
	require.Equal(t, privateKey, authentication.PrivateKey)
	trustedHosts, err := hostKeys.KnownHosts()
	require.NoError(t, err)
	require.Contains(t, string(trustedHosts), "ssh-ed25519")
}

func TestLocalCredentialProviderResolvesPassword(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "legacy-password"), []byte("secret with spaces\n"), 0o600))
	provider, err := NewLocalCredentialProvider(root, map[string]CredentialConfig{
		"legacy": {Type: "password", Username: "root", PasswordFile: "legacy-password"},
	})
	require.NoError(t, err)
	credential, err := provider.Resolve("legacy")
	require.NoError(t, err)
	require.Equal(t, "root", credential.Username)
	authentication, ok := credential.Authentication.(*PasswordAuthentication)
	require.True(t, ok)
	require.Equal(t, []byte("secret with spaces"), authentication.Password)
	credential.Clear()
	require.Equal(t, make([]byte, len("secret with spaces")), authentication.Password)
}

func TestSSHPreparerRequiresSSHPassForPassword(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "password"), []byte("secret\n"), 0o600))
	knownHosts := filepath.Join(t.TempDir(), "known_hosts")
	require.NoError(t, os.WriteFile(knownHosts, []byte("server ssh-ed25519 AAAATest\n"), 0o644))
	provider, err := NewLocalCredentialProvider(root, map[string]CredentialConfig{
		"legacy": {Type: "password", Username: "root", PasswordFile: "password"},
	})
	require.NoError(t, err)
	hostKeys, err := NewFileHostKeyProvider(knownHosts)
	require.NoError(t, err)
	preparer := NewSSHPreparer(provider, hostKeys, WithSSHPassBinary(filepath.Join(t.TempDir(), "missing")))
	require.ErrorContains(t, preparer.Validate(), "sshpass")
}

func TestLocalCredentialProviderRejectsUnsafeConfiguration(t *testing.T) {
	tests := []struct {
		name        string
		root        string
		credentials map[string]CredentialConfig
		wantError   string
	}{
		{name: "根目录必须是绝对路径", root: "credentials", credentials: map[string]CredentialConfig{
			"prod": {Username: "deploy", PrivateKeyFile: "key"},
		}, wantError: "绝对路径"},
		{name: "引用名称非法", root: t.TempDir(), credentials: map[string]CredentialConfig{
			"../prod": {Username: "deploy", PrivateKeyFile: "key"},
		}, wantError: "引用非法"},
		{name: "私钥路径不能越界", root: t.TempDir(), credentials: map[string]CredentialConfig{
			"prod": {Username: "deploy", PrivateKeyFile: "../key"},
		}, wantError: "文件名"},
		{name: "用户名不能包含空格", root: t.TempDir(), credentials: map[string]CredentialConfig{
			"prod": {Username: "bad user", PrivateKeyFile: "key"},
		}, wantError: "username"},
		{name: "密码凭据必须使用密码文件", root: t.TempDir(), credentials: map[string]CredentialConfig{
			"prod": {Type: "password", Username: "root", PrivateKeyFile: "key"},
		}, wantError: "password_file"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewLocalCredentialProvider(test.root, test.credentials)
			require.ErrorContains(t, err, test.wantError)
		})
	}
}

func TestLocalCredentialProviderRejectsUnsafeFiles(t *testing.T) {
	root := t.TempDir()
	writeTestPrivateKey(t, root, "wide-key", 0o644)
	knownHosts := filepath.Join(t.TempDir(), "known_hosts")
	require.NoError(t, os.WriteFile(knownHosts, []byte("server key\n"), 0o644))
	provider, err := NewLocalCredentialProvider(root, map[string]CredentialConfig{
		"prod": {Username: "deploy", PrivateKeyFile: "wide-key"},
	})
	require.NoError(t, err)
	require.ErrorContains(t, provider.Validate(), "权限不安全")

	_, err = provider.Resolve("missing")
	require.ErrorContains(t, err, "不存在")

	outside := t.TempDir()
	writeTestPrivateKey(t, outside, "outside-key", 0o600)
	require.NoError(t, os.Symlink(filepath.Join(outside, "outside-key"), filepath.Join(root, "linked-key")))
	linkedProvider, err := NewLocalCredentialProvider(root, map[string]CredentialConfig{
		"linked": {Username: "deploy", PrivateKeyFile: "linked-key"},
	})
	require.NoError(t, err)
	_, err = linkedProvider.Resolve("linked")
	require.ErrorContains(t, err, "超出凭据根目录")
}

func writeTestPrivateKey(t *testing.T, directory, name string, mode os.FileMode) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	content := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	require.NoError(t, os.WriteFile(filepath.Join(directory, name), content, mode))
	return content
}
