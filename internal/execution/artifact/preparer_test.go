package artifact

// 准备器测试覆盖默认层、具名层和缓存目录生命周期。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	artifactv1 "github.com/Duke1616/etask/api/proto/gen/etask/artifact/v1"
	artifactarchive "github.com/Duke1616/etask/internal/artifact/archive"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestValidateArtifactLayers(t *testing.T) {
	defaultLayer := validRef("")
	namedLayer := validRef("ops_common")
	other := proto.Clone(namedLayer).(*artifactv1.ArtifactRef)
	other.ReleaseId = 3
	testCases := []struct {
		name      string
		refs      []*artifactv1.ArtifactRef
		wantError string
	}{
		{name: "允许默认层与多个具名层", refs: []*artifactv1.ArtifactRef{defaultLayer, namedLayer}},
		{name: "拒绝空引用", refs: []*artifactv1.ArtifactRef{nil}, wantError: "空制品引用"},
		{name: "拒绝重复默认层", refs: []*artifactv1.ArtifactRef{defaultLayer, defaultLayer}, wantError: "重复的默认制品层"},
		{name: "拒绝重复挂载名称", refs: []*artifactv1.ArtifactRef{namedLayer, other}, wantError: "重复的制品挂载名称"},
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
	namedRef := proto.Clone(defaultRef).(*artifactv1.ArtifactRef)
	namedRef.ReleaseId = 2
	namedRef.MountName = "ops_common"
	client, closeServer := newArtifactClient(t, archive)
	defer closeServer()

	cacheDir := t.TempDir()
	runtime := NewRuntime(Config{Dir: cacheDir})
	prepared, err := runtime.Prepare(t.Context(), client, []*artifactv1.ArtifactRef{defaultRef, namedRef})
	require.NoError(t, err)
	roots := prepared.Roots()
	require.NotEmpty(t, roots.Default)
	require.Equal(t, roots.Default, roots.Named["ops_common"])
	require.Empty(t, roots.Dependencies)
	require.NoError(t, prepared.Close())
	require.DirExists(t, roots.Default)

	entries, err := os.ReadDir(filepath.Join(cacheDir, "tmp"))
	require.NoError(t, err)
	require.Empty(t, entries)
}

func validRef(namespace string) *artifactv1.ArtifactRef {
	return &artifactv1.ArtifactRef{
		ReleaseId: 1, MountName: namespace,
		Digest: strings.Repeat("a", 64), BlobChecksum: strings.Repeat("b", 64), Size: 1,
		Format: artifactarchive.Format, FormatVersion: artifactarchive.FormatVersion,
	}
}
