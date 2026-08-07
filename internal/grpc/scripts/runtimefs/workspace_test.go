package runtimefs

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Duke1616/etask/internal/grpc/scripts/engine"
	"github.com/Duke1616/etask/sdk/executor"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceFactoryCreate(t *testing.T) {
	type state struct {
		factory   *WorkspaceFactory
		workspace engine.Workspace
		system    string
		project   string
	}
	testCases := []struct {
		name       string
		before     func(t *testing.T, state *state)
		after      func(t *testing.T, state *state)
		artifacts  func(state *state) executor.ArtifactRoots
		assertions func(t *testing.T, state *state)
	}{
		{
			name:      "普通任务使用独立工作区",
			artifacts: func(_ *state) executor.ArtifactRoots { return executor.ArtifactRoots{} },
		},
		{
			name: "制品任务使用明确挂载目录",
			before: func(t *testing.T, state *state) {
				state.system = t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(state.system, "python"), 0o750))
				require.NoError(t, os.MkdirAll(filepath.Join(state.system, "third_party"), 0o750))
			},
			artifacts: func(state *state) executor.ArtifactRoots {
				return executor.ArtifactRoots{Default: state.system}
			},
			assertions: func(t *testing.T, state *state) {
				require.NoFileExists(t, filepath.Join(state.workspace.ProgramRoot(), "third_party"))
				mounted, err := os.Lstat(filepath.Join(state.workspace.ProgramRoot(), "system"))
				require.NoError(t, err)
				require.NotZero(t, mounted.Mode()&os.ModeSymlink)
				requireEnvironment(t, state.workspace.Environment(), "ETASK_SYSTEM_ROOT", filepath.Join(state.workspace.ProgramRoot(), "system"))
			},
		},
		{
			name: "SYSTEM 制品可以不包含 Python 目录",
			before: func(t *testing.T, state *state) {
				state.system = t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(state.system, "common.sh"), nil, 0o440))
			},
			artifacts: func(state *state) executor.ArtifactRoots {
				return executor.ArtifactRoots{Default: state.system}
			},
		},
		{
			name: "具名制品在工作区内组合为兼容依赖目录",
			before: func(t *testing.T, state *state) {
				state.project = createPythonArtifact(t, "project")
			},
			artifacts: func(state *state) executor.ArtifactRoots {
				return executor.ArtifactRoots{Named: map[string]string{"ops_common": state.project}}
			},
			assertions: func(t *testing.T, state *state) {
				dependencies := filepath.Join(state.workspace.ProgramRoot(), "dependencies")
				requireEnvironment(t, state.workspace.Environment(), "ETASK_DEPENDENCIES_ROOT", dependencies)
				require.FileExists(t, filepath.Join(dependencies, "python", "ops_common", "private", "util.py"))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			current := &state{}
			if tc.before != nil {
				tc.before(t, current)
			}
			factory, err := NewWorkspaceFactory(WorkspaceConfig{Dir: t.TempDir()}, NewHostWorkspaceAccess())
			require.NoError(t, err)
			current.factory = factory
			current.workspace, err = factory.Create(engine.WorkspaceOptions{
				ExecutionID: 42, Extension: ".sh", Program: inlineProgram("echo ok\n"), Artifacts: tc.artifacts(current),
			})
			require.NoError(t, err)
			defer func() { require.NoError(t, current.workspace.Close()) }()
			if tc.after != nil {
				defer tc.after(t, current)
			}
			require.True(t, filepath.IsAbs(current.workspace.ProgramRoot()))
			require.True(t, filepath.IsAbs(current.workspace.EntryPoint()))
			if tc.assertions != nil {
				tc.assertions(t, current)
			}
		})
	}
}

func TestWorkspaceFactoryMaterializesProjectProgram(t *testing.T) {
	source := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(source, "playbooks"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(source, "playbooks", "deploy.yml"), []byte("---\n"), 0o440))
	factory, err := NewWorkspaceFactory(WorkspaceConfig{Dir: t.TempDir()}, NewHostWorkspaceAccess())
	require.NoError(t, err)

	workspace, err := factory.Create(engine.WorkspaceOptions{
		ExecutionID: 42,
		Program: &executor.Program{
			Kind:    executor.ProgramKindProject,
			Project: &executor.ProjectProgram{Root: source, EntryPoint: "playbooks/deploy.yml"},
		},
	})
	require.NoError(t, err)
	defer func() { require.NoError(t, workspace.Close()) }()

	require.Equal(t, "project", filepath.Base(workspace.ProgramRoot()))
	require.Equal(t, filepath.Join(workspace.ProgramRoot(), "playbooks", "deploy.yml"), workspace.EntryPoint())
	requireEnvironment(t, workspace.Environment(), "ETASK_PROJECT_ROOT", workspace.ProgramRoot())
}

func TestWorkspaceFactoryRejectsEscapingProjectEntryPoint(t *testing.T) {
	factory, err := NewWorkspaceFactory(WorkspaceConfig{Dir: t.TempDir()}, NewHostWorkspaceAccess())
	require.NoError(t, err)
	_, err = factory.Create(engine.WorkspaceOptions{
		ExecutionID: 42,
		Program: &executor.Program{
			Kind:    executor.ProgramKindProject,
			Project: &executor.ProjectProgram{Root: t.TempDir(), EntryPoint: "../deploy.yml"},
		},
	})
	require.ErrorContains(t, err, "入口路径非法")
}

func TestPythonArtifactNamespaces(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("当前环境未安装 python3")
	}
	testCases := []struct {
		name      string
		before    func(t *testing.T) executor.ArtifactRoots
		code      string
		want      string
		wantError bool
	}{
		{
			name: "SYSTEM 与租户制品使用独立命名空间",
			before: func(t *testing.T) executor.ArtifactRoots {
				return executor.ArtifactRoots{
					Default: createPythonArtifact(t, "system"),
					Named:   map[string]string{"ops_common": createPythonArtifact(t, "project")},
				}
			},
			code: "from ops_common.private import util as project\nfrom etask.private import util as system\nprint(project.VALUE + ':' + system.VALUE)\n",
			want: "project:system",
		},
		{
			name: "SYSTEM 混合制品从根目录导入 Python 模块",
			before: func(t *testing.T) executor.ArtifactRoots {
				return executor.ArtifactRoots{Default: createFlatPythonArtifact(t)}
			},
			code: "from etask.third_party.base.want_result import want_result\nwant_result(status='success')\n",
			want: "success",
		},
		{
			name: "租户内多个制品库按英文名隔离",
			before: func(t *testing.T) executor.ArtifactRoots {
				return executor.ArtifactRoots{Named: map[string]string{
					"ops_common": createPythonArtifact(t, "ops"), "db_common": createPythonArtifact(t, "db"),
				}}
			},
			code: "from ops_common.private import util as ops\nfrom db_common.private import util as db\nprint(ops.VALUE + ':' + db.VALUE)\n",
			want: "ops:db",
		},
		{
			name: "租户制品可以引用 SYSTEM 和其他租户制品",
			before: func(t *testing.T) executor.ArtifactRoots {
				system := createPythonArtifact(t, "system")
				database := createPythonArtifact(t, "db")
				operations := createPythonArtifactFiles(t, map[string]string{
					"bridge.py": "from etask.private import util as system\n" +
						"from db_common.private import util as database\n" +
						"VALUE = system.VALUE + ':' + database.VALUE\n",
				})
				return executor.ArtifactRoots{
					Default: system,
					Named: map[string]string{
						"ops_common": operations, "db_common": database,
					},
				}
			},
			code: "from ops_common.private.bridge import VALUE\nprint(VALUE)\n",
			want: "system:db",
		},
		{
			name: "SYSTEM 模块支持包内相对引用",
			before: func(t *testing.T) executor.ArtifactRoots {
				return executor.ArtifactRoots{Default: createPythonArtifactFiles(t, map[string]string{
					"util.py":   "VALUE = 'system'\n",
					"bridge.py": "from .util import VALUE\nRESULT = VALUE + ':internal'\n",
				})}
			},
			code: "from etask.private.bridge import RESULT\nprint(RESULT)\n",
			want: "system:internal",
		},
		{
			name: "SYSTEM 模块不泄漏到顶层命名空间",
			before: func(t *testing.T) executor.ArtifactRoots {
				return executor.ArtifactRoots{Default: createPythonArtifact(t, "system")}
			},
			code: "import private\n", wantError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			factory, factoryErr := NewWorkspaceFactory(
				WorkspaceConfig{Dir: t.TempDir()}, NewHostWorkspaceAccess(),
			)
			require.NoError(t, factoryErr)
			workspace, createErr := factory.Create(engine.WorkspaceOptions{
				ExecutionID: 1, Extension: ".py", Program: inlineProgram(tc.code), Artifacts: tc.before(t),
			})
			require.NoError(t, createErr)
			defer func() { require.NoError(t, workspace.Close()) }()
			command := exec.Command(python, workspace.EntryPoint())
			command.Dir = workspace.ProgramRoot()
			command.Env = workspace.Environment()
			output, runErr := command.CombinedOutput()
			if tc.wantError {
				require.Error(t, runErr)
				return
			}
			require.NoError(t, runErr, string(output))
			require.Equal(t, tc.want, strings.TrimSpace(string(output)))
		})
	}
}

func TestWorkspaceRejectsInvalidNamedArtifact(t *testing.T) {
	factory, err := NewWorkspaceFactory(WorkspaceConfig{Dir: t.TempDir()}, NewHostWorkspaceAccess())
	require.NoError(t, err)
	_, err = factory.Create(engine.WorkspaceOptions{
		ExecutionID: 1,
		Extension:   ".sh",
		Program:     inlineProgram("echo ok\n"),
		Artifacts: executor.ArtifactRoots{
			Named: map[string]string{"../outside": t.TempDir()},
		},
	})
	require.ErrorContains(t, err, "挂载名称非法")
}

func inlineProgram(code string) *executor.Program {
	return &executor.Program{
		Kind:   executor.ProgramKindInline,
		Inline: &executor.InlineProgram{Code: code},
	}
}

func requireEnvironment(t *testing.T, environment []string, key, want string) {
	t.Helper()
	prefix := key + "="
	found := 0
	for _, item := range environment {
		if strings.HasPrefix(item, prefix) {
			found++
			require.Equal(t, want, strings.TrimPrefix(item, prefix))
		}
	}
	require.Equal(t, 1, found)
}

func createPythonArtifact(t *testing.T, value string) string {
	return createPythonArtifactFiles(t, map[string]string{"util.py": "VALUE = '" + value + "'\n"})
}

func createPythonArtifactFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	packageDir := filepath.Join(root, "python", "private")
	require.NoError(t, os.MkdirAll(packageDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(packageDir, "__init__.py"), nil, 0o440))
	for name, code := range files {
		require.NoError(t, os.WriteFile(filepath.Join(packageDir, name), []byte(code), 0o440))
	}
	return root
}

func createFlatPythonArtifact(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	packageDir := filepath.Join(root, "third_party", "base")
	require.NoError(t, os.MkdirAll(packageDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(packageDir, "want_result.py"),
		[]byte("def want_result(**kwargs): print(kwargs['status'])\n"), 0o440))
	return root
}
