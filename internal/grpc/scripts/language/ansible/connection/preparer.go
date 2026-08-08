// Package connection 为 Ansible 执行准备凭据和受信任的 SSH 连接材料。
package connection

import (
	"fmt"
	"maps"
	"os/exec"
	"slices"
	"strings"

	"github.com/Duke1616/etask/internal/grpc/scripts/engine"
	"github.com/Duke1616/etask/internal/grpc/scripts/language/ansible/inventory"
)

const inventoryCredentialReference = inventory.CredentialReferenceVariable

// Preparer 为 Ansible 准备一次执行所需的 SSH 连接变量和临时文件。
type Preparer interface {
	// References 返回可供任务选择的非敏感凭据引用。
	References() []string
	// Prepare 根据任务默认凭据和 inventory 映射生成逐主机连接变量。
	Prepare(workspace engine.Workspace, request Request) (Preparation, error)
	// Validate 校验所有连接 Provider 的配置和运行依赖。
	Validate() error
}

// Preparation 保存连接变量以及需要在执行日志中隐藏的敏感值。
type Preparation struct {
	Variables   map[string]any
	SecretMasks []string
}

// Request 描述一次执行使用的默认凭据和静态 inventory。
type Request struct {
	DefaultReference string
	InventoryFile    string
}

// SSHPreparer 将 Provider 返回的敏感材料物化到单次执行工作区。
type SSHPreparer struct {
	credentials CredentialProvider
	hostKeys    HostKeyProvider
	inventory   inventory.Resolver
	sshPass     string
}

// Option 配置 SSH 连接准备服务的运行依赖。
type Option func(*SSHPreparer)

// WithSSHPassBinary 设置密码认证使用的 sshpass 命令路径。
func WithSSHPassBinary(binary string) Option {
	return func(preparer *SSHPreparer) {
		preparer.sshPass = strings.TrimSpace(binary)
	}
}

// WithInventoryCredentialResolver 设置逐主机凭据引用解析器。
func WithInventoryCredentialResolver(resolver inventory.Resolver) Option {
	return func(preparer *SSHPreparer) {
		preparer.inventory = resolver
	}
}

// NewSSHPreparer 创建 SSH 连接准备服务。
func NewSSHPreparer(credentials CredentialProvider, hostKeys HostKeyProvider,
	options ...Option) *SSHPreparer {
	preparer := &SSHPreparer{
		credentials: credentials, hostKeys: hostKeys,
		inventory: inventory.StaticResolver{}, sshPass: "sshpass",
	}
	for _, option := range options {
		option(preparer)
	}
	return preparer
}

// References 返回可供任务选择的凭据引用。
func (p *SSHPreparer) References() []string {
	if p == nil || p.credentials == nil {
		return nil
	}
	return p.credentials.References()
}

// Validate 校验凭据与主机信任 Provider；未登记凭据时保持向后兼容。
func (p *SSHPreparer) Validate() error {
	if p == nil || p.credentials == nil || len(p.credentials.References()) == 0 {
		return nil
	}
	if err := p.credentials.Validate(); err != nil {
		return err
	}
	if p.hostKeys == nil {
		return fmt.Errorf("Ansible 已配置 SSH 凭据，但缺少主机信任 Provider")
	}
	if err := p.hostKeys.Validate(); err != nil {
		return err
	}
	for _, reference := range p.credentials.References() {
		credential, err := p.credentials.Resolve(reference)
		if err != nil {
			return err
		}
		password := credential.Authentication != nil && credential.Authentication.Kind() == AuthenticationKindPassword
		credential.Clear()
		if password {
			binary := p.sshPass
			if binary == "" {
				binary = "sshpass"
			}
			if _, err = exec.LookPath(binary); err != nil {
				return fmt.Errorf("Ansible 密码凭据需要 sshpass，但当前 Agent 不可用: %w", err)
			}
			break
		}
	}
	return nil
}

// Prepare 解析任务默认凭据和 inventory 映射，并返回仅供当前执行使用的连接变量。
func (p *SSHPreparer) Prepare(workspace engine.Workspace, request Request) (Preparation, error) {
	defaultReference := strings.TrimSpace(request.DefaultReference)
	plan, err := p.resolveInventory(request.InventoryFile)
	if err != nil {
		return Preparation{}, err
	}
	if len(plan.References) == 0 {
		return p.prepareDefault(workspace, defaultReference)
	}
	return p.preparePerHost(workspace, plan, defaultReference)
}

// resolveInventory 只在配置了 inventory 凭据标记时读取解析器，避免影响普通 inventory。
func (p *SSHPreparer) resolveInventory(inventoryFile string) (inventory.Plan, error) {
	if p == nil || p.inventory == nil || strings.TrimSpace(inventoryFile) == "" {
		return inventory.Plan{}, nil
	}
	return p.inventory.Resolve(inventoryFile)
}

// prepareDefault 处理整批主机共用一个凭据的兼容路径。
func (p *SSHPreparer) prepareDefault(workspace engine.Workspace, reference string) (Preparation, error) {
	if reference == "" {
		return Preparation{}, nil
	}
	commonArgs, err := p.prepareHostTrust(workspace)
	if err != nil {
		return Preparation{}, err
	}
	connection, err := p.prepareReference(workspace, reference, commonArgs)
	if err != nil {
		return Preparation{}, err
	}
	return Preparation{
		Variables: stringVariables(connection.variables), SecretMasks: connection.secretMasks,
	}, nil
}

// preparePerHost 每种凭据只物化一次，再通过 inventory_hostname 选择连接变量。
func (p *SSHPreparer) preparePerHost(workspace engine.Workspace, plan inventory.Plan,
	defaultReference string) (Preparation, error) {
	if p == nil || p.credentials == nil {
		return Preparation{}, fmt.Errorf("inventory 配置了 %s，但 Agent 未配置凭据 Provider", inventoryCredentialReference)
	}
	commonArgs, err := p.prepareHostTrust(workspace)
	if err != nil {
		return Preparation{}, err
	}
	hostConnections := make(map[string]map[string]string, len(plan.Hosts))
	cache := newPreparedCredentialCache()
	var unbound []string
	for _, host := range plan.Hosts {
		reference := credentialReferenceForHost(plan, host, defaultReference)
		if reference == "" {
			unbound = append(unbound, host)
			continue
		}
		connection, err := cache.getOrPrepare(p, workspace, reference, commonArgs)
		if err != nil {
			return Preparation{}, fmt.Errorf("准备主机 %q 的凭据失败: %w", host, err)
		}
		hostConnections[host] = connection.variables
	}
	if len(unbound) > 0 {
		return Preparation{}, unboundHostsError(unbound)
	}
	return Preparation{
		Variables: perHostConnectionVariables(hostConnections), SecretMasks: cache.sortedSecretMasks(),
	}, nil
}

// MapVariable 是逐主机连接变量在 Extra Vars 中使用的内部映射名。
const MapVariable = "__etask_connections"

// ReservedVariable 判断 Extra Vars 是否会覆盖受连接模块管理的凭据或 SSH 参数。
func ReservedVariable(key string) bool {
	switch key {
	case "ansible_password", "ansible_pass", "ansible_ssh_pass", "ansible_ssh_password",
		"ansible_private_key", "ansible_ssh_private_key", "ansible_private_key_file",
		"ansible_ssh_private_key_file", "ansible_ssh_private_key_passphrase",
		"ansible_ssh_common_args", "ansible_become_password", "ansible_become_pass",
		MapVariable, inventory.CredentialReferenceVariable:
		return true
	default:
		return false
	}
}

type preparedCredentialCache struct {
	connections map[string]preparedCredential
	secretMasks map[string]struct{}
}

func newPreparedCredentialCache() *preparedCredentialCache {
	return &preparedCredentialCache{
		connections: make(map[string]preparedCredential), secretMasks: make(map[string]struct{}),
	}
}

// getOrPrepare 保证同一凭据只读取和物化一次，主机规模不会放大敏感文件操作。
func (c *preparedCredentialCache) getOrPrepare(preparer *SSHPreparer, workspace engine.Workspace,
	reference, commonArgs string) (preparedCredential, error) {
	if connection, exists := c.connections[reference]; exists {
		return connection, nil
	}
	connection, err := preparer.prepareReference(workspace, reference, commonArgs)
	if err != nil {
		return preparedCredential{}, err
	}
	completePerHostVariables(connection.variables)
	c.connections[reference] = connection
	for _, mask := range connection.secretMasks {
		c.secretMasks[mask] = struct{}{}
	}
	return connection, nil
}

func (c *preparedCredentialCache) sortedSecretMasks() []string {
	return slices.Sorted(maps.Keys(c.secretMasks))
}

func credentialReferenceForHost(plan inventory.Plan, host, defaultReference string) string {
	if reference := strings.TrimSpace(plan.References[host]); reference != "" {
		return reference
	}
	return defaultReference
}

func completePerHostVariables(variables map[string]string) {
	if _, exists := variables["ansible_password"]; !exists {
		variables["ansible_password"] = ""
	}
	if _, exists := variables["ansible_ssh_private_key_file"]; !exists {
		variables["ansible_ssh_private_key_file"] = ""
	}
}

func unboundHostsError(hosts []string) error {
	const maximumReportedHosts = 5
	reported := hosts
	if len(reported) > maximumReportedHosts {
		reported = reported[:maximumReportedHosts]
	}
	return fmt.Errorf(
		"inventory 启用了逐主机凭据，但以下主机没有 %s 且任务未设置默认 credential_ref: %s",
		inventoryCredentialReference, strings.Join(reported, ", "),
	)
}

func perHostConnectionVariables(hostConnections map[string]map[string]string) map[string]any {
	// Extra Vars 具有最高变量优先级，使用 inventory_hostname 为每台主机选择对应连接材料。
	return map[string]any{
		MapVariable:                    hostConnections,
		"ansible_user":                 "{{ " + MapVariable + "[inventory_hostname].ansible_user }}",
		"ansible_password":             "{{ " + MapVariable + "[inventory_hostname].ansible_password }}",
		"ansible_ssh_private_key_file": "{{ " + MapVariable + "[inventory_hostname].ansible_ssh_private_key_file }}",
		"ansible_ssh_common_args":      "{{ " + MapVariable + "[inventory_hostname].ansible_ssh_common_args }}",
	}
}

func (p *SSHPreparer) prepareHostTrust(workspace engine.Workspace) (string, error) {
	if p == nil || p.credentials == nil {
		return "", fmt.Errorf("Ansible 未配置本地凭据 Provider")
	}
	if p.hostKeys == nil {
		return "", fmt.Errorf("Ansible 未配置主机信任 Provider")
	}
	knownHosts, err := p.hostKeys.KnownHosts()
	if err != nil {
		return "", fmt.Errorf("读取 Ansible known_hosts 失败: %w", err)
	}
	knownHostsFile, err := workspace.WriteFile("ansible-known-hosts", knownHosts, 0o644)
	if err != nil {
		return "", fmt.Errorf("写入 Ansible 临时 known_hosts 失败: %w", err)
	}
	return fmt.Sprintf(
		"-o StrictHostKeyChecking=yes -o UserKnownHostsFile=%s -o IdentitiesOnly=yes",
		shellQuote(knownHostsFile),
	), nil
}

var _ Preparer = (*SSHPreparer)(nil)
