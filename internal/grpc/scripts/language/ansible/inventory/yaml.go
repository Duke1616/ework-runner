package inventory

import (
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"
)

type yamlInventoryGroup struct {
	Hosts    map[string]map[string]any     `yaml:"hosts"`
	Vars     map[string]any                `yaml:"vars"`
	Children map[string]yamlInventoryGroup `yaml:"children"`
}

func parseYAMLInventory(content []byte) (parsedInventory, error) {
	var groups map[string]yamlInventoryGroup
	if err := yaml.Unmarshal(content, &groups); err != nil {
		return parsedInventory{}, err
	}

	parsed := newParsedInventory()
	if all, exists := groups["all"]; exists {
		if err := walkYAMLGroup(&parsed, "all", all, 0, nil); err != nil {
			return parsedInventory{}, err
		}
	}

	// 即使没有显式写入 all.children，Ansible 也会让其他顶层组继承 all。
	allCandidates, err := yamlAllCandidates(groups["all"])
	if err != nil {
		return parsedInventory{}, err
	}
	reachable := make(map[string]struct{})
	if all, exists := groups["all"]; exists {
		collectYAMLChildren(all.Children, reachable)
	}
	for name, group := range groups {
		if name == "all" {
			continue
		}
		if _, nested := reachable[name]; nested {
			continue
		}
		if err := walkYAMLGroup(&parsed, name, group, 1, allCandidates); err != nil {
			return parsedInventory{}, err
		}
	}
	return parsed, nil
}

func yamlAllCandidates(all yamlInventoryGroup) ([]credentialCandidate, error) {
	reference, err := inventoryReference(all.Vars)
	if err != nil {
		return nil, fmt.Errorf("组 all: %w", err)
	}
	if reference == "" {
		return nil, nil
	}
	return []credentialCandidate{{
		reference: reference, priority: 0, source: "组 all",
	}}, nil
}

func collectYAMLChildren(groups map[string]yamlInventoryGroup, result map[string]struct{}) {
	for name, group := range groups {
		if _, exists := result[name]; exists {
			continue
		}
		result[name] = struct{}{}
		collectYAMLChildren(group.Children, result)
	}
}

func walkYAMLGroup(parsed *parsedInventory, name string, group yamlInventoryGroup, depth int,
	inherited []credentialCandidate) error {
	candidates, err := yamlGroupCandidates(name, group, depth, inherited)
	if err != nil {
		return err
	}
	if err := addYAMLHosts(parsed, group, candidates); err != nil {
		return err
	}
	for childName, child := range group.Children {
		if err := walkYAMLGroup(parsed, childName, child, depth+1, candidates); err != nil {
			return err
		}
	}
	return nil
}

func yamlGroupCandidates(name string, group yamlInventoryGroup, depth int,
	inherited []credentialCandidate) ([]credentialCandidate, error) {
	candidates := append([]credentialCandidate(nil), inherited...)
	reference, err := inventoryReference(group.Vars)
	if err != nil {
		return nil, fmt.Errorf("组 %q: %w", name, err)
	}
	if reference == "" {
		return candidates, nil
	}
	return append(candidates, credentialCandidate{
		reference: reference, priority: depth, source: "组 " + name,
	}), nil
}

func addYAMLHosts(parsed *parsedInventory, group yamlInventoryGroup,
	inherited []credentialCandidate) error {
	for host, variables := range group.Hosts {
		hostCandidates, err := yamlHostCandidates(host, variables, inherited)
		if err != nil {
			return fmt.Errorf("主机 %q: %w", host, err)
		}
		if err := parsed.addHost(host, hostCandidates...); err != nil {
			return err
		}
	}
	return nil
}

func yamlHostCandidates(host string, variables map[string]any,
	inherited []credentialCandidate) ([]credentialCandidate, error) {
	candidates := append([]credentialCandidate(nil), inherited...)
	reference, err := inventoryReference(variables)
	if err != nil {
		return nil, err
	}
	if reference == "" {
		return candidates, nil
	}
	// 主机显式配置永远覆盖任意深度的组变量。
	return append(candidates, credentialCandidate{
		reference: reference, priority: maxInt, source: "主机 " + host,
	}), nil
}

func inventoryReference(variables map[string]any) (string, error) {
	if variables == nil {
		return "", nil
	}
	value, ok := variables[CredentialReferenceVariable]
	if !ok || value == nil {
		return "", nil
	}
	reference, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s 必须是字符串", CredentialReferenceVariable)
	}
	return strings.TrimSpace(reference), nil
}
