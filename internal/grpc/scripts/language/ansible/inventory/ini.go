package inventory

import (
	"bufio"
	"bytes"
	"fmt"
	"slices"
	"strings"
)

type iniInventoryGroup struct {
	hosts     map[string]map[string]string
	variables map[string]string
	children  []string
}

func parseINIInventory(content []byte) (parsedInventory, error) {
	groups := make(map[string]*iniInventoryGroup)
	group := ensureINIGroup(groups, "ungrouped")
	sectionKind := "hosts"
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 64*1024), maximumInventorySize)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if isINISection(line) {
			name, kind, err := parseINISection(line)
			if err != nil {
				return parsedInventory{}, fmt.Errorf("第 %d 行 section 非法", lineNumber)
			}
			group = ensureINIGroup(groups, name)
			sectionKind = kind
			continue
		}
		if err := parseINIEntry(groups, group, sectionKind, line); err != nil {
			return parsedInventory{}, fmt.Errorf("第 %d 行%s", lineNumber, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return parsedInventory{}, err
	}
	return resolveINIInventory(groups)
}

func isINISection(line string) bool {
	return strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]")
}

func parseINISection(line string) (string, string, error) {
	name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
	kind := "hosts"
	if base, suffix, found := strings.Cut(name, ":"); found {
		name, kind = strings.TrimSpace(base), strings.TrimSpace(suffix)
	}
	if name == "" || (kind != "hosts" && kind != "vars" && kind != "children") {
		return "", "", fmt.Errorf("section 非法")
	}
	return name, kind, nil
}

func parseINIEntry(groups map[string]*iniInventoryGroup, group *iniInventoryGroup, kind, line string) error {
	switch kind {
	case "vars":
		return parseINIGroupVariable(group, line)
	case "children":
		return parseINIChildGroup(groups, group, line)
	default:
		return parseINIHost(group, line)
	}
}

func parseINIGroupVariable(group *iniInventoryGroup, line string) error {
	key, value, found := strings.Cut(line, "=")
	if !found || strings.TrimSpace(key) == "" {
		return fmt.Errorf("组变量非法")
	}
	group.variables[strings.TrimSpace(key)] = normalizeINIValue(value)
	return nil
}

func parseINIChildGroup(groups map[string]*iniInventoryGroup, group *iniInventoryGroup, line string) error {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return fmt.Errorf("子组定义非法")
	}
	group.children = append(group.children, fields[0])
	ensureINIGroup(groups, fields[0])
	return nil
}

func parseINIHost(group *iniInventoryGroup, line string) error {
	fields, err := splitINIFields(line)
	if err != nil || len(fields) == 0 {
		return fmt.Errorf("主机定义非法")
	}
	variables := make(map[string]string)
	for _, field := range fields[1:] {
		if strings.HasPrefix(field, "#") || strings.HasPrefix(field, ";") {
			break
		}
		key, value, found := strings.Cut(field, "=")
		if found {
			variables[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	group.hosts[fields[0]] = variables
	return nil
}

// splitINIFields 拆分 INI 主机行，保留引号内的空格且不执行 Shell 展开。
func splitINIFields(line string) ([]string, error) {
	var fields []string
	var current strings.Builder
	var quote rune
	escaped := false
	started := false
	flush := func() {
		if !started {
			return
		}
		fields = append(fields, current.String())
		current.Reset()
		started = false
	}
	for _, char := range line {
		if escaped {
			current.WriteRune(char)
			started = true
			escaped = false
			continue
		}
		if char == '\\' && quote != '\'' {
			escaped = true
			started = true
			continue
		}
		if quote != 0 {
			if char == quote {
				quote = 0
			} else {
				current.WriteRune(char)
			}
			continue
		}
		switch char {
		case '\'', '"':
			quote = char
			started = true
		case ' ', '\t', '\r', '\n':
			flush()
		case '\x00':
			return nil, fmt.Errorf("主机定义包含空字符")
		default:
			current.WriteRune(char)
			started = true
		}
	}
	if escaped || quote != 0 {
		return nil, fmt.Errorf("主机定义包含未完成的转义或引号")
	}
	flush()
	return fields, nil
}

func normalizeINIValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') ||
		(value[0] == '"' && value[len(value)-1] == '"')) {
		return value[1 : len(value)-1]
	}
	return value
}

func ensureINIGroup(groups map[string]*iniInventoryGroup, name string) *iniInventoryGroup {
	if group := groups[name]; group != nil {
		return group
	}
	group := &iniInventoryGroup{
		hosts: make(map[string]map[string]string), variables: make(map[string]string),
	}
	groups[name] = group
	return group
}

func resolveINIInventory(groups map[string]*iniInventoryGroup) (parsedInventory, error) {
	roots, err := findINIRootGroups(groups)
	if err != nil {
		return parsedInventory{}, err
	}
	parsed := newParsedInventory()
	allCandidates := iniAllCandidates(groups["all"])
	for _, root := range roots {
		depth, inherited := iniRootInheritance(root, allCandidates)
		if err := walkINIGroup(&parsed, groups, root, depth, inherited, make(map[string]bool)); err != nil {
			return parsedInventory{}, err
		}
	}
	return parsed, nil
}

func findINIRootGroups(groups map[string]*iniInventoryGroup) ([]string, error) {
	children := make(map[string]struct{})
	for _, group := range groups {
		for _, child := range group.children {
			children[child] = struct{}{}
		}
	}
	roots := make([]string, 0, len(groups))
	for name := range groups {
		if _, child := children[name]; !child {
			roots = append(roots, name)
		}
	}
	slices.Sort(roots)
	if len(groups) > 0 && len(roots) == 0 {
		return nil, fmt.Errorf("inventory 分组存在循环引用")
	}
	return roots, nil
}

func iniRootInheritance(root string, allCandidates []credentialCandidate) (int, []credentialCandidate) {
	// Ansible 会让独立顶层组隐式继承 all；all 自身则从零层开始计算。
	if root == "all" {
		return 0, nil
	}
	return 1, allCandidates
}

func iniAllCandidates(group *iniInventoryGroup) []credentialCandidate {
	if group == nil {
		return nil
	}
	if reference := strings.TrimSpace(group.variables[CredentialReferenceVariable]); reference != "" {
		return []credentialCandidate{{reference: reference, priority: 0, source: "组 all"}}
	}
	return nil
}

func walkINIGroup(parsed *parsedInventory, groups map[string]*iniInventoryGroup, name string, depth int,
	inherited []credentialCandidate, visiting map[string]bool) error {
	// visiting 只记录当前递归路径，既能发现环，也允许同一子组被多个父组引用。
	if visiting[name] {
		return fmt.Errorf("inventory 分组 %q 存在循环引用", name)
	}
	visiting[name] = true
	defer delete(visiting, name)
	group := groups[name]
	candidates := iniGroupCandidates(name, group, depth, inherited)
	if err := addINIHosts(parsed, group, candidates); err != nil {
		return err
	}
	for _, child := range group.children {
		if err := walkINIGroup(parsed, groups, child, depth+1, candidates, visiting); err != nil {
			return err
		}
	}
	return nil
}

func iniGroupCandidates(name string, group *iniInventoryGroup, depth int,
	inherited []credentialCandidate) []credentialCandidate {
	candidates := append([]credentialCandidate(nil), inherited...)
	reference := strings.TrimSpace(group.variables[CredentialReferenceVariable])
	if reference == "" {
		return candidates
	}
	return append(candidates, credentialCandidate{
		reference: reference, priority: depth, source: "组 " + name,
	})
}

func addINIHosts(parsed *parsedInventory, group *iniInventoryGroup, inherited []credentialCandidate) error {
	for host, variables := range group.hosts {
		hostCandidates := append([]credentialCandidate(nil), inherited...)
		// 主机显式配置永远覆盖任意深度的组变量。
		if reference := strings.TrimSpace(variables[CredentialReferenceVariable]); reference != "" {
			hostCandidates = append(hostCandidates, credentialCandidate{
				reference: reference, priority: maxInt, source: "主机 " + host,
			})
		}
		if err := parsed.addHost(host, hostCandidates...); err != nil {
			return err
		}
	}
	return nil
}
