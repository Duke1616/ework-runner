package ansible

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPlaybookOptionsCommandArgs(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "hosts.ini"), []byte("localhost\n"), 0o440))
	options := PlaybookOptions{
		Inventory: "hosts.ini", Limit: "web", Tags: []string{"deploy", "deploy", "restart"},
		SkipTags: []string{"database"}, Check: true, Diff: true, Become: true, BecomeUser: "root",
		Forks: 20, Verbosity: 3, ExtraArgs: []string{"--syntax-check"},
	}
	args, err := options.commandArgs(root)
	require.NoError(t, err)
	require.Equal(t, []string{
		"--inventory", filepath.Join(root, "hosts.ini"), "--limit", "web",
		"--tags", "deploy,restart", "--skip-tags", "database", "--check", "--diff",
		"--become", "--become-user", "root", "--forks", "20", "-vvv", "--syntax-check",
	}, args)
}

func TestParsePlaybookOptionsFromMetadataParams(t *testing.T) {
	options, err := parsePlaybookOptions(map[string]string{
		"inventory": "inventory/staging.yml", "limit": "web", "tags": "deploy,restart",
		"skip_tags": "database", "check": "true", "diff": "true", "become": "true",
		"become_user": "root", "forks": "10", "verbosity": "2",
		"extra_args": `--syntax-check --start-at-task "Deploy application"`,
	})

	require.NoError(t, err)
	require.Equal(t, PlaybookOptions{
		Inventory: "inventory/staging.yml", Limit: "web", Tags: []string{"deploy", "restart"},
		SkipTags: []string{"database"}, Check: true, Diff: true, Become: true,
		BecomeUser: "root", Forks: 10, Verbosity: 2,
		ExtraArgs: []string{"--syntax-check", "--start-at-task", "Deploy application"},
	}, options)
}

func TestSplitExtraArgsRejectsMalformedInput(t *testing.T) {
	_, err := splitExtraArgs(`--start-at-task "Deploy application`)
	require.ErrorContains(t, err, "未闭合")

	_, err = splitExtraArgs("--syntax-check \\")
	require.ErrorContains(t, err, "未完成的转义")
}

func TestPlaybookOptionsRejectInvalidValues(t *testing.T) {
	root := t.TempDir()
	outsideInventory := filepath.Join(t.TempDir(), "hosts.ini")
	require.NoError(t, os.WriteFile(outsideInventory, []byte("localhost\n"), 0o440))
	require.NoError(t, os.Symlink(outsideInventory, filepath.Join(root, "linked-hosts.ini")))
	require.NoError(t, os.Mkdir(filepath.Join(root, "inventory"), 0o750))
	testCases := []struct {
		name      string
		options   PlaybookOptions
		wantError string
	}{
		{name: "inventory 越界", options: PlaybookOptions{Inventory: "../hosts"}, wantError: "合法相对路径"},
		{name: "inventory 不存在", options: PlaybookOptions{Inventory: "hosts.ini"}, wantError: "访问"},
		{name: "inventory 不是文件", options: PlaybookOptions{Inventory: "inventory"}, wantError: "不是普通文件"},
		{name: "inventory 符号链接越界", options: PlaybookOptions{Inventory: "linked-hosts.ini"}, wantError: "超出项目目录"},
		{name: "forks 超限", options: PlaybookOptions{Forks: 101}, wantError: "forks"},
		{name: "verbosity 超限", options: PlaybookOptions{Verbosity: 5}, wantError: "verbosity"},
		{name: "提权用户缺少 become", options: PlaybookOptions{BecomeUser: "root"}, wantError: "become_user"},
		{name: "扩展参数不能包含分隔符", options: PlaybookOptions{ExtraArgs: []string{"--"}}, wantError: "扩展参数非法"},
		{name: "其他参数不能覆盖 limit", options: PlaybookOptions{ExtraArgs: []string{"--limit=web"}}, wantError: "结构化配置"},
		{name: "其他参数不能覆盖 extra vars", options: PlaybookOptions{ExtraArgs: []string{"-e@vars.json"}}, wantError: "结构化配置"},
		{name: "禁止交互式 SSH 密码", options: PlaybookOptions{ExtraArgs: []string{"-k"}}, wantError: "结构化配置"},
		{name: "禁止覆盖 SSH 私钥", options: PlaybookOptions{ExtraArgs: []string{"--private-key=/tmp/key"}}, wantError: "结构化配置"},
		{name: "禁止关闭受控 SSH 参数", options: PlaybookOptions{ExtraArgs: []string{"--ssh-common-args=-o StrictHostKeyChecking=no"}}, wantError: "结构化配置"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := testCase.options.commandArgs(root)
			require.ErrorContains(t, err, testCase.wantError)
		})
	}
}
