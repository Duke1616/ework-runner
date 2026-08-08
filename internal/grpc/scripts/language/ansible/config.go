package ansible

import (
	"github.com/Duke1616/etask/internal/grpc/scripts/engine"
	"github.com/Duke1616/etask/internal/grpc/scripts/language/ansible/connection"
)

// Config 配置 Ansible Handler 及其 Agent 本地连接能力。
type Config struct {
	Enabled        bool                                   `mapstructure:"enabled" yaml:"enabled"`
	Binary         string                                 `mapstructure:"binary" yaml:"binary"`
	SSHPassBinary  string                                 `mapstructure:"sshpass_binary" yaml:"sshpass_binary"`
	CredentialRoot string                                 `mapstructure:"credential_root" yaml:"credential_root"`
	KnownHostsFile string                                 `mapstructure:"known_hosts_file" yaml:"known_hosts_file"`
	Credentials    map[string]connection.CredentialConfig `mapstructure:"credentials" yaml:"credentials"`
}

// IsEnabled 返回是否注册 Ansible Handler。
func (c Config) IsEnabled() bool { return c.Enabled }

// Build 构造 Ansible Adapter 及其 SSH 连接依赖。
func (c Config) Build() (engine.Adapter, error) {
	credentialProvider, err := connection.NewLocalCredentialProvider(c.CredentialRoot, c.Credentials)
	if err != nil {
		return nil, err
	}
	hostKeyProvider, err := connection.NewFileHostKeyProvider(c.KnownHostsFile)
	if err != nil {
		return nil, err
	}
	connections := connection.NewSSHPreparer(
		credentialProvider, hostKeyProvider, connection.WithSSHPassBinary(c.SSHPassBinary),
	)
	return New(c.Binary, WithSSHConnectionPreparer(connections)), nil
}

var _ engine.AdapterFactory = Config{}
