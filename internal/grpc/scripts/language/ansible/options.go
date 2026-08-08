package ansible

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const (
	maximumForks     = 100
	maximumVerbosity = 4
)

// PlaybookOptions 描述用户可配置的 ansible-playbook 运行选项。
type PlaybookOptions struct {
	Inventory  string   `json:"inventory,omitempty"`
	Limit      string   `json:"limit,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	SkipTags   []string `json:"skip_tags,omitempty"`
	Check      bool     `json:"check,omitempty"`
	Diff       bool     `json:"diff,omitempty"`
	Become     bool     `json:"become,omitempty"`
	BecomeUser string   `json:"become_user,omitempty"`
	Forks      int      `json:"forks,omitempty"`
	Verbosity  int      `json:"verbosity,omitempty"`
	ExtraArgs  []string `json:"extra_args,omitempty"`
}

func parsePlaybookOptions(params map[string]string) (PlaybookOptions, error) {
	tags, err := parseCommaList("tags", params["tags"])
	if err != nil {
		return PlaybookOptions{}, err
	}
	skipTags, err := parseCommaList("skip_tags", params["skip_tags"])
	if err != nil {
		return PlaybookOptions{}, err
	}
	extraArgs, err := splitExtraArgs(params["extra_args"])
	if err != nil {
		return PlaybookOptions{}, err
	}
	check, err := parseBool("check", params["check"])
	if err != nil {
		return PlaybookOptions{}, err
	}
	diff, err := parseBool("diff", params["diff"])
	if err != nil {
		return PlaybookOptions{}, err
	}
	become, err := parseBool("become", params["become"])
	if err != nil {
		return PlaybookOptions{}, err
	}
	forks, err := parseInt("forks", params["forks"])
	if err != nil {
		return PlaybookOptions{}, err
	}
	verbosity, err := parseInt("verbosity", params["verbosity"])
	if err != nil {
		return PlaybookOptions{}, err
	}
	return PlaybookOptions{
		Inventory: params["inventory"], Limit: params["limit"], Tags: tags, SkipTags: skipTags,
		Check: check, Diff: diff, Become: become, BecomeUser: params["become_user"],
		Forks: forks, Verbosity: verbosity, ExtraArgs: extraArgs,
	}, nil
}

func parseCommaList(name, raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	return normalizedList(name, strings.Split(raw, ","))
}

// splitExtraArgs 将用户输入拆成独立 argv，不执行 Shell 展开或命令替换。
func splitExtraArgs(raw string) ([]string, error) {
	var result []string
	var current strings.Builder
	var quote rune
	escaped := false
	started := false
	flush := func() {
		if started {
			result = append(result, current.String())
			current.Reset()
			started = false
		}
	}
	for _, char := range raw {
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
			return nil, fmt.Errorf("Ansible 扩展参数包含空字符")
		default:
			current.WriteRune(char)
			started = true
		}
	}
	if escaped {
		return nil, fmt.Errorf("Ansible 扩展参数以未完成的转义结尾")
	}
	if quote != 0 {
		return nil, fmt.Errorf("Ansible 扩展参数包含未闭合的引号")
	}
	flush()
	return result, nil
}

func parseBool(name, raw string) (bool, error) {
	if strings.TrimSpace(raw) == "" {
		return false, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("解析 Ansible 参数 %s 失败: 必须是布尔值: %w", name, err)
	}
	return value, nil
}

func parseInt(name, raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("解析 Ansible 参数 %s 失败: 必须是整数: %w", name, err)
	}
	return value, nil
}

func (o PlaybookOptions) commandArgs(programRoot string) ([]string, error) {
	inventory := ""
	if strings.TrimSpace(o.Inventory) != "" {
		resolved, err := validateInventory(programRoot, o.Inventory)
		if err != nil {
			return nil, err
		}
		inventory = resolved
	}
	return o.commandArgsWithInventory(inventory)
}

// commandArgsWithInventory 使用已经校验过的 inventory 路径构造命令参数。
func (o PlaybookOptions) commandArgsWithInventory(inventory string) ([]string, error) {
	if o.Forks < 0 || o.Forks > maximumForks {
		return nil, fmt.Errorf("Ansible forks 必须在 1 到 %d 之间，0 表示使用默认值", maximumForks)
	}
	if o.Verbosity < 0 || o.Verbosity > maximumVerbosity {
		return nil, fmt.Errorf("Ansible verbosity 必须在 0 到 %d 之间", maximumVerbosity)
	}
	if strings.TrimSpace(o.BecomeUser) != "" && !o.Become {
		return nil, fmt.Errorf("Ansible become_user 要求同时启用 become")
	}
	args := make([]string, 0, 20+len(o.ExtraArgs))
	if inventory != "" {
		args = append(args, "--inventory", inventory)
	}
	if value, err := normalizedText("limit", o.Limit); err != nil {
		return nil, err
	} else if value != "" {
		args = append(args, "--limit", value)
	}
	if tags, err := normalizedList("tags", o.Tags); err != nil {
		return nil, err
	} else if len(tags) > 0 {
		args = append(args, "--tags", strings.Join(tags, ","))
	}
	if tags, err := normalizedList("skip_tags", o.SkipTags); err != nil {
		return nil, err
	} else if len(tags) > 0 {
		args = append(args, "--skip-tags", strings.Join(tags, ","))
	}
	if o.Check {
		args = append(args, "--check")
	}
	if o.Diff {
		args = append(args, "--diff")
	}
	if o.Become {
		args = append(args, "--become")
		if value, err := normalizedText("become_user", o.BecomeUser); err != nil {
			return nil, err
		} else if value != "" {
			args = append(args, "--become-user", value)
		}
	}
	if o.Forks > 0 {
		args = append(args, "--forks", fmt.Sprintf("%d", o.Forks))
	}
	if o.Verbosity > 0 {
		args = append(args, "-"+strings.Repeat("v", o.Verbosity))
	}
	extra, err := validateExtraArgs(o.ExtraArgs)
	if err != nil {
		return nil, err
	}
	return append(args, extra...), nil
}

func validateInventory(programRoot, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\\\x00") || strings.HasPrefix(value, "/") ||
		path.Clean(value) != value || value == "." || value == ".." || strings.HasPrefix(value, "../") {
		return "", fmt.Errorf("Ansible inventory 必须是项目内的合法相对路径: %q", value)
	}
	target := filepath.Join(programRoot, filepath.FromSlash(value))
	relative, err := filepath.Rel(programRoot, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) ||
		filepath.IsAbs(relative) {
		return "", fmt.Errorf("Ansible inventory 超出项目目录: %q", value)
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", fmt.Errorf("访问 Ansible inventory 失败: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("Ansible inventory 不是普通文件: %q", value)
	}
	resolvedRoot, err := filepath.EvalSymlinks(programRoot)
	if err != nil {
		return "", fmt.Errorf("解析 Ansible 项目目录失败: %w", err)
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", fmt.Errorf("解析 Ansible inventory 失败: %w", err)
	}
	resolvedRelative, err := filepath.Rel(resolvedRoot, resolvedTarget)
	if err != nil || resolvedRelative == ".." ||
		strings.HasPrefix(resolvedRelative, ".."+string(filepath.Separator)) || filepath.IsAbs(resolvedRelative) {
		return "", fmt.Errorf("Ansible inventory 超出项目目录: %q", value)
	}
	return target, nil
}

func normalizedText(name, value string) (string, error) {
	value = strings.TrimSpace(value)
	if strings.ContainsRune(value, '\x00') {
		return "", fmt.Errorf("Ansible %s 包含空字符", name)
	}
	return value, nil
}

func normalizedList(name string, values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value, err := normalizedText(name, value)
		if err != nil {
			return nil, err
		}
		if value != "" && !slices.Contains(result, value) {
			result = append(result, value)
		}
	}
	return result, nil
}

func validateExtraArgs(values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || strings.ContainsRune(value, '\x00') || value == "--" {
			return nil, fmt.Errorf("Ansible 扩展参数非法: %q", value)
		}
		if strings.HasPrefix(value, "-") && managedOption(value) {
			return nil, fmt.Errorf("Ansible 参数 %q 已有结构化配置，不能在其他参数中重复指定", value)
		}
		result = append(result, value)
	}
	return result, nil
}

func managedOption(value string) bool {
	name, _, _ := strings.Cut(value, "=")
	managedLong := []string{
		"--inventory", "--inventory-file", "--limit", "--subset", "--tags", "--skip-tags",
		"--check", "--diff", "--become", "--become-user", "--forks", "--extra-vars", "--verbose",
		"--private-key", "--key-file", "--ask-pass", "--ask-become-pass", "--become-password-file",
		"--ssh-common-args", "--ssh-extra-args", "--scp-extra-args", "--sftp-extra-args",
		"--vault-id", "--vault-password-file",
	}
	if slices.Contains(managedLong, name) {
		return true
	}
	if strings.HasPrefix(value, "--") {
		return false
	}
	for _, prefix := range []string{"-i", "-l", "-t", "-C", "-D", "-b", "-f", "-e", "-k", "-K"} {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return len(value) > 1 && strings.Trim(value[1:], "v") == ""
}
