package artifact

// 准备器测试覆盖默认层、具名层和缓存目录生命周期。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	artifactarchive "github.com/Duke1616/etask/internal/artifact/archive"
	executorartifact "github.com/Duke1616/etask/sdk/executor/artifact"
	artifactgrpc "github.com/Duke1616/etask/sdk/executor/artifact/grpc"
	"github.com/stretchr/testify/require"
)

func TestValidateArtifactLayers(t *testing.T) {
	defaultLayer := validRef("")
	namedLayer := validRef("ops_common")
	other := namedLayer
	other.ReleaseID = 3
	source := sourceRef(defaultLayer)
	testCases := []struct {
		name      string
		source    *executorartifact.SourceRef
		refs      []executorartifact.Ref
		wantError string
	}{
		{name: "允许来源、默认层与多个具名层", source: &source, refs: []executorartifact.Ref{defaultLayer, namedLayer}},
		{name: "拒绝空引用", refs: []executorartifact.Ref{{}}, wantError: "发布 ID 非法"},
		{name: "拒绝重复默认层", refs: []executorartifact.Ref{defaultLayer, defaultLayer}, wantError: "重复的默认制品层"},
		{name: "拒绝重复挂载名称", refs: []executorartifact.Ref{namedLayer, other}, wantError: "重复的制品挂载名称"},
		{name: "拒绝空项目源码", source: &executorartifact.SourceRef{}, wantError: "源码 ID 非法"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseLayerSet(tc.source, tc.refs)
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestRuntimePrepare(t *testing.T) {
	testCases := []struct {
		name       string
		setup      func(t *testing.T) (executorartifact.Downloader, *executorartifact.SourceRef, []executorartifact.Ref, func())
		assertions func(t *testing.T, prepared executorartifact.PreparedArtifacts, cacheDir string)
		wantError  string
	}{
		{
			name: "准备默认层与具名层并保证目录不可变及临时目录清空",
			setup: func(t *testing.T) (executorartifact.Downloader, *executorartifact.SourceRef, []executorartifact.Ref, func()) {
				archive, defaultRef := buildTestRef(t, "python/private/util.py", "VALUE = 1\n")
				namedRef := defaultRef
				namedRef.ReleaseID = 2
				namedRef.MountName = "ops_common"
				client, closeServer := newArtifactClient(t, archive)
				return artifactgrpc.NewDownloader(client), nil, []executorartifact.Ref{defaultRef, namedRef}, closeServer
			},
			assertions: func(t *testing.T, prepared executorartifact.PreparedArtifacts, cacheDir string) {
				roots := prepared.Roots()
				require.NotEmpty(t, roots.Default)
				require.Equal(t, roots.Default, roots.Named["ops_common"])
				require.DirExists(t, roots.Default)

				entries, err := os.ReadDir(filepath.Join(cacheDir, "tmp"))
				require.NoError(t, err)
				require.Empty(t, entries)
			},
		},
		{
			name: "准备项目源码并正确返回 SourceRoot 物理目录",
			setup: func(t *testing.T) (executorartifact.Downloader, *executorartifact.SourceRef, []executorartifact.Ref, func()) {
				archive, artifactRef := buildTestRef(t, "playbooks/deploy.yml", "---\n")
				source := sourceRef(artifactRef)
				client, closeServer := newArtifactClient(t, archive)
				return artifactgrpc.NewDownloader(client), &source, nil, closeServer
			},
			assertions: func(t *testing.T, prepared executorartifact.PreparedArtifacts, cacheDir string) {
				require.FileExists(t, filepath.Join(prepared.SourceRoot(), "playbooks", "deploy.yml"))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dl, source, refs, closeFn := tc.setup(t)
			defer closeFn()

			cacheDir := t.TempDir()
			runtime := NewRuntime(Config{Dir: cacheDir})
			prepared, err := runtime.Prepare(t.Context(), dl, source, refs)
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				return
			}
			require.NoError(t, err)
			tc.assertions(t, prepared, cacheDir)
		})
	}
}

func validRef(namespace string) executorartifact.Ref {
	return executorartifact.Ref{
		ReleaseID: 1, MountName: namespace,
		Digest: strings.Repeat("a", 64), BlobChecksum: strings.Repeat("b", 64), Size: 1,
		Format: artifactarchive.Format, FormatVersion: artifactarchive.FormatVersion,
	}
}

func sourceRef(ref executorartifact.Ref) executorartifact.SourceRef {
	return executorartifact.SourceRef{
		SourceID: ref.ReleaseID, Digest: ref.Digest, BlobChecksum: ref.BlobChecksum,
		Size: ref.Size, Format: ref.Format, FormatVersion: ref.FormatVersion,
	}
}
