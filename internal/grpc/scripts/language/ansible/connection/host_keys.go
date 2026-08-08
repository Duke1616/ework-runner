package connection

import (
	"fmt"
	"path/filepath"
	"strings"
)

// FileHostKeyProvider 从 Agent 本地文件读取 SSH 主机信任数据。
type FileHostKeyProvider struct {
	path string
}

// NewFileHostKeyProvider 创建基于 OpenSSH known_hosts 文件的信任提供器。
func NewFileHostKeyProvider(path string) (*FileHostKeyProvider, error) {
	path = strings.TrimSpace(path)
	if path != "" && !filepath.IsAbs(path) {
		return nil, fmt.Errorf("Ansible known_hosts_file 必须是绝对路径: %q", path)
	}
	return &FileHostKeyProvider{path: path}, nil
}

// KnownHosts 读取受保护的 known_hosts 文件内容。
func (p *FileHostKeyProvider) KnownHosts() ([]byte, error) {
	if p.path == "" {
		return nil, fmt.Errorf("未配置 Ansible known_hosts_file")
	}
	return readProtectedFile(p.path, knownHostsFilePolicy)
}

// Validate 校验 known_hosts 文件路径、权限和大小。
func (p *FileHostKeyProvider) Validate() error {
	if _, err := p.KnownHosts(); err != nil {
		return fmt.Errorf("校验 Ansible known_hosts_file 失败: %w", err)
	}
	return nil
}

var _ HostKeyProvider = (*FileHostKeyProvider)(nil)
