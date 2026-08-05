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
	testCases := []struct {
		name      string
		refs      []executorartifact.Ref
		wantError string
	}{
		{name: "允许默认层与多个具名层", refs: []executorartifact.Ref{defaultLayer, namedLayer}},
		{name: "拒绝空引用", refs: []executorartifact.Ref{{}}, wantError: "发布 ID 非法"},
		{name: "拒绝重复默认层", refs: []executorartifact.Ref{defaultLayer, defaultLayer}, wantError: "重复的默认制品层"},
		{name: "拒绝重复挂载名称", refs: []executorartifact.Ref{namedLayer, other}, wantError: "重复的制品挂载名称"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseLayerSet(tc.refs)
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestRuntimePrepareReturnsImmutableNamedLayers(t *testing.T) {
	archive, defaultRef := buildTestRef(t, "python/private/util.py", "VALUE = 1\n")
	namedRef := defaultRef
	namedRef.ReleaseID = 2
	namedRef.MountName = "ops_common"
	client, closeServer := newArtifactClient(t, archive)
	defer closeServer()

	cacheDir := t.TempDir()
	runtime := NewRuntime(Config{Dir: cacheDir})
	prepared, err := runtime.Prepare(t.Context(), artifactgrpc.NewDownloader(client),
		[]executorartifact.Ref{defaultRef, namedRef})
	require.NoError(t, err)
	roots := prepared.Roots()
	require.NotEmpty(t, roots.Default)
	require.Equal(t, roots.Default, roots.Named["ops_common"])
	require.NoError(t, prepared.Close())
	require.DirExists(t, roots.Default)

	entries, err := os.ReadDir(filepath.Join(cacheDir, "tmp"))
	require.NoError(t, err)
	require.Empty(t, entries)
}

func validRef(namespace string) executorartifact.Ref {
	return executorartifact.Ref{
		ReleaseID: 1, MountName: namespace,
		Digest: strings.Repeat("a", 64), BlobChecksum: strings.Repeat("b", 64), Size: 1,
		Format: artifactarchive.Format, FormatVersion: artifactarchive.FormatVersion,
	}
}
