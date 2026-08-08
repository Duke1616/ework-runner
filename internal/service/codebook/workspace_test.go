package codebook

import (
	"context"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/stretchr/testify/require"
)

type workspaceSourceStub struct {
	nodes []domain.Codebook
}

func (s workspaceSourceStub) Tree(context.Context, int64) ([]domain.Codebook, error) {
	return s.nodes, nil
}

func (s workspaceSourceStub) SystemTree(context.Context, int64) ([]domain.Codebook, error) {
	return s.nodes, nil
}

type workspaceArtifactStub struct {
	contents []domain.ArtifactContent
	code     string
}

func (s workspaceArtifactStub) ActiveContents(_ context.Context, sourceProjectID int64) ([]domain.ArtifactContent, error) {
	contents := make([]domain.ArtifactContent, 0, len(s.contents))
	for _, content := range s.contents {
		if content.Release.Scope != domain.CodebookScopeTenant || content.Release.ProjectID != sourceProjectID {
			contents = append(contents, content)
		}
	}
	return contents, nil
}

func (s workspaceArtifactStub) ReadFile(context.Context, int64, string, string) (string, error) {
	return s.code, nil
}

func TestWorkspaceServiceTree(t *testing.T) {
	testCases := []struct {
		name     string
		source   []domain.Codebook
		contents []domain.ArtifactContent
		assert   func(*testing.T, []domain.WorkspaceNode, error)
	}{
		{
			name: "后端生成三层完整运行路径",
			source: []domain.Codebook{
				{ID: 1, ProjectID: 9, Name: "scripts", Kind: domain.CodebookKindDirectory, Scope: domain.CodebookScopeTenant},
				{ID: 2, ProjectID: 9, ParentID: 1, Name: "smoke.sh", Kind: domain.CodebookKindFile, Scope: domain.CodebookScopeTenant},
			},
			contents: []domain.ArtifactContent{
				{
					Release: domain.ArtifactRelease{ID: 10, Scope: domain.CodebookScopeSystem, Digest: "system"},
					Files:   []domain.ArtifactManifestFile{{Path: "private/utils.sh"}},
				},
				{
					Release: domain.ArtifactRelease{ID: 11, Scope: domain.CodebookScopeTenant, ProjectID: 20, Namespace: "ops_common", Digest: "tenant"},
					Files:   []domain.ArtifactManifestFile{{Path: "private/utils.sh"}},
				},
				{
					Release: domain.ArtifactRelease{ID: 12, Scope: domain.CodebookScopeTenant, ProjectID: 9, Namespace: "self_library", Digest: "self"},
					Files:   []domain.ArtifactManifestFile{{Path: "private/self.sh"}},
				},
			},
			assert: func(t *testing.T, nodes []domain.WorkspaceNode, err error) {
				require.NoError(t, err)
				require.Len(t, nodes, 3)
				require.Equal(t, "scripts/smoke.sh", nodes[0].Children[0].Children[0].RuntimePath)
				require.Equal(t, "system/private/utils.sh", nodes[1].Children[0].Children[0].RuntimePath)
				require.Equal(t, "dependencies/ops_common/private/utils.sh",
					nodes[2].Children[0].Children[0].Children[0].RuntimePath)
				require.Len(t, nodes[2].Children, 1)
				require.True(t, nodes[1].Readonly)
				require.True(t, nodes[2].Children[0].Readonly)
			},
		},
		{
			name:   "未发布制品仍返回稳定空层",
			source: nil,
			assert: func(t *testing.T, nodes []domain.WorkspaceNode, err error) {
				require.NoError(t, err)
				require.Equal(t, []string{"", "system", "dependencies"},
					[]string{nodes[0].RuntimePath, nodes[1].RuntimePath, nodes[2].RuntimePath})
				require.Empty(t, nodes[1].Children)
				require.Empty(t, nodes[2].Children)
			},
		},
		{
			name: "拒绝不可达的循环节点",
			source: []domain.Codebook{
				{ID: 1, ParentID: 2, ProjectID: 9, Name: "a", Kind: domain.CodebookKindDirectory},
				{ID: 2, ParentID: 1, ProjectID: 9, Name: "b", Kind: domain.CodebookKindDirectory},
			},
			assert: func(t *testing.T, _ []domain.WorkspaceNode, err error) {
				require.ErrorContains(t, err, "无法从根目录访问")
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			service := NewWorkspaceService(workspaceSourceStub{nodes: testCase.source},
				workspaceArtifactStub{contents: testCase.contents})
			nodes, err := service.Tree(t.Context(), 9)
			testCase.assert(t, nodes, err)
		})
	}
}

func TestWorkspaceServiceSystemTree(t *testing.T) {
	service := NewWorkspaceService(workspaceSourceStub{nodes: []domain.Codebook{
		{ID: 10, Name: "ansible", Kind: domain.CodebookKindDirectory, Scope: domain.CodebookScopeSystem},
		{ID: 11, ParentID: 10, Name: "playbooks", Kind: domain.CodebookKindDirectory, Scope: domain.CodebookScopeSystem},
		{ID: 12, ParentID: 11, Name: "deploy.yml", Kind: domain.CodebookKindFile, Scope: domain.CodebookScopeSystem},
	}}, workspaceArtifactStub{})

	nodes, err := service.SystemTree(t.Context(), 10)

	require.NoError(t, err)
	require.Len(t, nodes, 1)
	require.Equal(t, "ansible", nodes[0].Name)
	require.Equal(t, domain.CodebookScopeSystem, nodes[0].Scope)
	require.True(t, nodes[0].Readonly)
	require.Equal(t, int64(0), nodes[0].Children[0].ParentID)
	require.True(t, nodes[0].Children[0].Children[0].Readonly)
	require.Equal(t, "playbooks/deploy.yml", nodes[0].Children[0].Children[0].RuntimePath)
}

func TestWorkspaceServiceReadArtifactFile(t *testing.T) {
	service := NewWorkspaceService(nil, workspaceArtifactStub{
		code: "echo ok\n",
		contents: []domain.ArtifactContent{{
			Release: domain.ArtifactRelease{ID: 1, Scope: domain.CodebookScopeSystem, Digest: "digest"},
			Files:   []domain.ArtifactManifestFile{{Path: "scripts/test.sh"}},
		}},
	})
	code, err := service.ReadArtifactFile(t.Context(), 9, 1, "digest", "scripts/test.sh")
	require.NoError(t, err)
	require.Equal(t, "echo ok\n", code)

	_, err = service.ReadArtifactFile(t.Context(), 9, 2, "digest", "scripts/test.sh")
	require.ErrorContains(t, err, "不属于当前激活制品")
}
