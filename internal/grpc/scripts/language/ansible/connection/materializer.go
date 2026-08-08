package connection

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/Duke1616/etask/internal/grpc/scripts/engine"
)

type preparedCredential struct {
	variables   map[string]string
	secretMasks []string
}

// prepareReference 读取一个凭据，并把认证材料物化到本次执行工作区。
func (p *SSHPreparer) prepareReference(workspace engine.Workspace, reference,
	commonArgs string) (preparedCredential, error) {
	credential, err := p.credentials.Resolve(reference)
	if err != nil {
		return preparedCredential{}, err
	}
	defer credential.Clear()
	variables := map[string]string{
		"ansible_user": credential.Username, "ansible_ssh_common_args": commonArgs,
	}
	secretMasks, err := materializeAuthentication(workspace, reference, credential.Authentication, variables)
	if err != nil {
		return preparedCredential{}, err
	}
	return preparedCredential{variables: variables, secretMasks: secretMasks}, nil
}

func stringVariables(source map[string]string) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func materializeAuthentication(workspace engine.Workspace, reference string, authentication Authentication,
	variables map[string]string) ([]string, error) {
	switch auth := authentication.(type) {
	case *PrivateKeyAuthentication:
		if auth == nil {
			return nil, fmt.Errorf("Ansible 凭据缺少私钥认证材料")
		}
		privateKeyFile, err := workspace.WriteFile(
			"ansible-ssh-private-key-"+credentialFileSuffix(reference), auth.PrivateKey, 0o600,
		)
		if err != nil {
			return nil, fmt.Errorf("写入 Ansible 临时私钥失败: %w", err)
		}
		variables["ansible_ssh_private_key_file"] = privateKeyFile
		return nil, nil
	case *PasswordAuthentication:
		if auth == nil || len(auth.Password) == 0 {
			return nil, fmt.Errorf("Ansible 凭据缺少密码认证材料")
		}
		password := string(auth.Password)
		variables["ansible_password"] = password
		return []string{password}, nil
	case nil:
		return nil, fmt.Errorf("Ansible 凭据缺少认证材料")
	default:
		return nil, fmt.Errorf("Ansible 暂不支持认证类型 %q", authentication.Kind())
	}
}

func credentialFileSuffix(reference string) string {
	// 只使用非敏感引用的摘要，避免认证材料或未经约束的原始值进入路径。
	sum := sha256.Sum256([]byte(reference))
	return fmt.Sprintf("%x", sum[:6])
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
