package inventory

import (
	"bytes"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const (
	// CredentialReferenceVariable 是静态 inventory 中声明凭据引用的变量名。
	CredentialReferenceVariable = "etask_credential_ref"
	maximumInventorySize        = 8 << 20
	maxInt                      = int(^uint(0) >> 1)
)

var credentialReferencePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// ValidReference 判断凭据引用是否符合 inventory 与 Provider 共享的命名约束。
func ValidReference(reference string) bool {
	return credentialReferencePattern.MatchString(reference)
}

// Plan 描述静态 inventory 中的主机和非敏感凭据引用。
type Plan struct {
	Hosts      []string
	References map[string]string
}

// Resolver 从 inventory 中解析逐主机凭据引用。
type Resolver interface {
	// Resolve 解析 inventory 的主机分组继承关系并生成逐主机凭据计划。
	Resolve(inventoryFile string) (Plan, error)
}

// StaticResolver 支持 Ansible 静态 YAML 和 INI inventory。
type StaticResolver struct{}

// Resolve 根据文件扩展名解析静态 inventory。
func (StaticResolver) Resolve(inventoryFile string) (Plan, error) {
	content, err := readInventoryFile(inventoryFile)
	if err != nil {
		return Plan{}, err
	}
	// 没有 etask 标记时不解析，保持动态 inventory 和原有 inventory 变量兼容。
	if !bytes.Contains(content, []byte(CredentialReferenceVariable)) {
		return Plan{}, nil
	}

	var parsed parsedInventory
	switch strings.ToLower(filepath.Ext(inventoryFile)) {
	case ".yaml", ".yml":
		parsed, err = parseYAMLInventory(content)
	default:
		parsed, err = parseINIInventory(content)
	}
	if err != nil {
		return Plan{}, fmt.Errorf("解析 Ansible inventory 凭据引用失败: %w", err)
	}
	if len(parsed.hosts) == 0 {
		return Plan{}, fmt.Errorf("inventory 包含 %s，但没有解析到静态主机", CredentialReferenceVariable)
	}
	return parsed.resolve()
}

type parsedInventory struct {
	hosts      map[string]struct{}
	candidates map[string][]credentialCandidate
}

type credentialCandidate struct {
	reference string
	priority  int
	source    string
}

func newParsedInventory() parsedInventory {
	return parsedInventory{
		hosts: make(map[string]struct{}), candidates: make(map[string][]credentialCandidate),
	}
}

func (p *parsedInventory) addHost(host string, candidates ...credentialCandidate) error {
	host = strings.TrimSpace(host)
	if host == "" || strings.ContainsAny(host, "\x00\r\n\t ") {
		return fmt.Errorf("inventory 主机名非法: %q", host)
	}
	p.hosts[host] = struct{}{}
	for _, candidate := range candidates {
		if candidate.reference == "" {
			continue
		}
		if !ValidReference(candidate.reference) {
			return fmt.Errorf("%s 中的凭据引用非法: %q", candidate.source, candidate.reference)
		}
		p.candidates[host] = append(p.candidates[host], candidate)
	}
	return nil
}

// resolve 应用主机优先级，并拒绝同级来源之间的歧义。
func (p parsedInventory) resolve() (Plan, error) {
	hosts := slices.Sorted(maps.Keys(p.hosts))
	result := Plan{Hosts: hosts, References: make(map[string]string)}
	for _, host := range hosts {
		reference, err := selectCredentialReference(p.candidates[host])
		if err != nil {
			return Plan{}, fmt.Errorf("主机 %q: %w", host, err)
		}
		if reference != "" {
			result.References[host] = reference
		}
	}
	return result, nil
}

// selectCredentialReference 只保留最高优先级；同级不同引用无法确定意图，必须显式报错。
func selectCredentialReference(candidates []credentialCandidate) (string, error) {
	if len(candidates) == 0 {
		return "", nil
	}
	highest := candidates[0].priority
	references := map[string]struct{}{candidates[0].reference: {}}
	for _, candidate := range candidates[1:] {
		switch {
		case candidate.priority > highest:
			highest = candidate.priority
			references = map[string]struct{}{candidate.reference: {}}
		case candidate.priority == highest:
			references[candidate.reference] = struct{}{}
		}
	}
	if len(references) > 1 {
		values := slices.Sorted(maps.Keys(references))
		return "", fmt.Errorf("匹配到多个同优先级凭据: %s", strings.Join(values, ", "))
	}
	for reference := range references {
		return reference, nil
	}
	return "", nil
}

func readInventoryFile(name string) ([]byte, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maximumInventorySize {
		return nil, fmt.Errorf("inventory 文件类型或大小非法")
	}
	content, err := io.ReadAll(io.LimitReader(file, maximumInventorySize+1))
	if err != nil {
		return nil, err
	}
	if len(content) > maximumInventorySize {
		return nil, fmt.Errorf("inventory 文件类型或大小非法")
	}
	return content, nil
}

var _ Resolver = StaticResolver{}
