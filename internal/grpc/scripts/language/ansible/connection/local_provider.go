package connection

import (
	"bytes"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Duke1616/etask/internal/grpc/scripts/language/ansible/inventory"
	"golang.org/x/crypto/ssh"
)

const (
	credentialTypePrivateKey = "private_key"
	credentialTypePassword   = "password"
)

// CredentialConfig 描述 Agent 本地的一种 SSH 凭据。
type CredentialConfig struct {
	Type           string `mapstructure:"type" yaml:"type"`
	Username       string `mapstructure:"username" yaml:"username"`
	PrivateKeyFile string `mapstructure:"private_key_file" yaml:"private_key_file"`
	PasswordFile   string `mapstructure:"password_file" yaml:"password_file"`
}

// LocalCredentialProvider 从 Agent 本地受保护目录读取 SSH 凭据。
type LocalCredentialProvider struct {
	root        string
	credentials map[string]CredentialConfig
}

// NewLocalCredentialProvider 创建 Agent 本地 SSH 凭据提供器。
func NewLocalCredentialProvider(root string, credentials map[string]CredentialConfig) (*LocalCredentialProvider, error) {
	root = strings.TrimSpace(root)
	if len(credentials) > 0 && root == "" {
		return nil, fmt.Errorf("Ansible 凭据已配置，但 credential_root 为空")
	}
	if root != "" && !filepath.IsAbs(root) {
		return nil, fmt.Errorf("Ansible credential_root 必须是绝对路径: %q", root)
	}
	cloned := make(map[string]CredentialConfig, len(credentials))
	for reference, credential := range credentials {
		if err := validateCredentialConfig(reference, &credential); err != nil {
			return nil, err
		}
		cloned[reference] = credential
	}
	return &LocalCredentialProvider{root: root, credentials: cloned}, nil
}

// References 返回可供任务选择的非敏感凭据引用。
func (p *LocalCredentialProvider) References() []string {
	return slices.Sorted(maps.Keys(p.credentials))
}

// Validate 校验凭据根目录以及全部已登记凭据。
func (p *LocalCredentialProvider) Validate() error {
	if len(p.credentials) == 0 {
		return nil
	}
	if err := validateProtectedDirectory(p.root); err != nil {
		return fmt.Errorf("校验 Ansible credential_root 失败: %w", err)
	}
	for reference := range p.credentials {
		credential, err := p.Resolve(reference)
		if err != nil {
			return err
		}
		credential.Clear()
	}
	return nil
}

// Resolve 读取一个凭据引用并验证认证材料格式。
func (p *LocalCredentialProvider) Resolve(reference string) (Credential, error) {
	if !inventory.ValidReference(reference) {
		return Credential{}, fmt.Errorf("Ansible 凭据引用非法: %q", reference)
	}
	config, ok := p.credentials[reference]
	if !ok {
		return Credential{}, fmt.Errorf("Ansible 凭据不存在: %q", reference)
	}
	if config.Type == credentialTypePassword {
		password, err := readFileWithinRoot(p.root, config.PasswordFile, passwordFilePolicy)
		if err != nil {
			return Credential{}, fmt.Errorf("读取 Ansible 凭据 %q 密码失败: %w", reference, err)
		}
		trimmedPassword := append([]byte(nil), bytes.TrimRight(password, "\r\n")...)
		clearBytes(password)
		password = trimmedPassword
		if len(password) == 0 {
			clearBytes(password)
			return Credential{}, fmt.Errorf("Ansible 凭据 %q 的密码文件为空", reference)
		}
		return Credential{
			Username:       config.Username,
			Authentication: &PasswordAuthentication{Password: password},
		}, nil
	}
	privateKey, err := readFileWithinRoot(p.root, config.PrivateKeyFile, privateKeyFilePolicy)
	if err != nil {
		return Credential{}, fmt.Errorf("读取 Ansible 凭据 %q 失败: %w", reference, err)
	}
	if _, err = ssh.ParseRawPrivateKey(privateKey); err != nil {
		clearBytes(privateKey)
		return Credential{}, fmt.Errorf("Ansible 凭据 %q 不是可直接使用的 SSH 私钥（暂不支持带口令私钥）: %w", reference, err)
	}
	return Credential{
		Username:       config.Username,
		Authentication: &PrivateKeyAuthentication{PrivateKey: privateKey},
	}, nil
}

func validateCredentialConfig(reference string, credential *CredentialConfig) error {
	if !inventory.ValidReference(reference) {
		return fmt.Errorf("Ansible 凭据引用非法: %q", reference)
	}
	credential.Type = strings.ToLower(strings.TrimSpace(credential.Type))
	if credential.Type == "" {
		credential.Type = credentialTypePrivateKey
	}
	if credential.Type != credentialTypePrivateKey && credential.Type != credentialTypePassword {
		return fmt.Errorf("Ansible 凭据 %q 的 type 只支持 private_key 或 password", reference)
	}
	credential.Username = strings.TrimSpace(credential.Username)
	if credential.Username == "" || strings.ContainsAny(credential.Username, "\x00\r\n\t ") || strings.HasPrefix(credential.Username, "-") {
		return fmt.Errorf("Ansible 凭据 %q 的 username 非法", reference)
	}
	credential.PrivateKeyFile = strings.TrimSpace(credential.PrivateKeyFile)
	credential.PasswordFile = strings.TrimSpace(credential.PasswordFile)
	if credential.Type == credentialTypePrivateKey {
		if err := validateCredentialFile(reference, "private_key_file", credential.PrivateKeyFile); err != nil {
			return err
		}
		if credential.PasswordFile != "" {
			return fmt.Errorf("Ansible 凭据 %q 的 private_key 类型不能配置 password_file", reference)
		}
	} else {
		if err := validateCredentialFile(reference, "password_file", credential.PasswordFile); err != nil {
			return err
		}
		if credential.PrivateKeyFile != "" {
			return fmt.Errorf("Ansible 凭据 %q 的 password 类型不能配置 private_key_file", reference)
		}
	}
	return nil
}

func validateCredentialFile(reference, field, name string) error {
	if name == "" || name != filepath.Base(name) || strings.ContainsAny(name, "\x00/\\") || name == "." || name == ".." {
		return fmt.Errorf("Ansible 凭据 %q 的 %s 必须是 credential_root 下的文件名", reference, field)
	}
	return nil
}

var _ CredentialProvider = (*LocalCredentialProvider)(nil)
