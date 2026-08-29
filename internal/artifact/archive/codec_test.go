package archive

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
)

func TestCodecPackContext(t *testing.T) {
	testCases := []struct {
		name     string
		file     domain.ArtifactFile
		content  string
		readPath string
	}{
		{
			name: "流式打包并读取 Blob 大文件",
			file: domain.ArtifactFile{
				Path: "files/payload.bin", StorageType: domain.CodebookContentBlob,
				ObjectKey: "blob-key",
			},
			content:  strings.Repeat("payload", 1024),
			readPath: "files/payload.bin",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sum := sha256.Sum256([]byte(tc.content))
			tc.file.Size = int64(len(tc.content))
			tc.file.Hash = hex.EncodeToString(sum[:])

			codec := New(t.TempDir())
			packed, err := codec.PackContext(context.Background(), []domain.ArtifactFile{tc.file},
				func(_ context.Context, file domain.ArtifactFile) (io.ReadCloser, error) {
					require.Equal(t, tc.file.ObjectKey, file.ObjectKey)
					return io.NopCloser(strings.NewReader(tc.content)), nil
				},
			)
			require.NoError(t, err)
			t.Cleanup(func() { _ = os.Remove(packed.Path) })

			reader, err := os.Open(packed.Path)
			require.NoError(t, err)
			defer reader.Close()

			actual, err := New("").ReadFile(reader, packed.Digest, tc.readPath)
			require.NoError(t, err)
			require.Equal(t, tc.content, actual)
		})
	}
}

func TestCodecProducesDeterministicArtifact(t *testing.T) {
	codec := New(t.TempDir())
	files := []domain.ArtifactFile{
		{Path: "python/util.py", Code: "VALUE = 2\n"},
		{Path: "common.sh", Code: "echo common\n"},
	}

	first, err := codec.Pack(files)
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(first.Path) })
	second, err := codec.Pack([]domain.ArtifactFile{files[1], files[0]})
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(second.Path) })

	require.Equal(t, first.Digest, second.Digest)
	require.Equal(t, first.BlobChecksum, second.BlobChecksum)
	require.Equal(t, first.Size, second.Size)
	require.Equal(t, Format, first.Format)
	require.Equal(t, FormatVersion, first.FormatVersion)

	entries, manifest := readArtifact(t, first.Path)
	require.Equal(t, []string{ManifestPath, "common.sh", "python/util.py"}, entries)
	require.Equal(t, first.Digest, manifest.Digest)
	require.Len(t, manifest.Files, 2)
	require.Equal(t, "common.sh", manifest.Files[0].Path)
	require.Equal(t, "python/util.py", manifest.Files[1].Path)
}

func TestCodecRejectsInvalidFiles(t *testing.T) {
	codec := New(t.TempDir())
	testCases := []struct {
		name      string
		files     []domain.ArtifactFile
		wantError string
	}{
		{name: "empty", wantError: "没有可发布的文件"},
		{name: "path traversal", files: []domain.ArtifactFile{{Path: "../secret", Code: "x"}}, wantError: "超出根目录"},
		{name: "non canonical path", files: []domain.ArtifactFile{{Path: "lib/../secret", Code: "x"}}, wantError: "规范相对路径"},
		{name: "reserved path", files: []domain.ArtifactFile{{Path: ".etask/manifest.json", Code: "x"}}, wantError: "保留目录"},
		{name: "duplicate", files: []domain.ArtifactFile{{Path: "a.py", Code: "x"}, {Path: "a.py", Code: "y"}}, wantError: "重复路径"},
		{name: "hash mismatch", files: []domain.ArtifactFile{{Path: "a.py", Code: "x", Hash: "invalid"}}, wantError: "校验和与版本记录不一致"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := codec.Pack(tc.files)
			require.ErrorContains(t, err, tc.wantError)
		})
	}
}

func TestCodecExtract(t *testing.T) {
	codec := New(t.TempDir())
	packed, err := codec.Pack([]domain.ArtifactFile{{Path: "common.sh", Code: "echo ok\n"}})
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(packed.Path) })

	testCases := []struct {
		name      string
		meta      Metadata
		limits    ExtractLimits
		wantError string
		check     func(t *testing.T, target string)
	}{
		{
			name:   "解压普通文件并设置只读权限",
			meta:   Metadata{Digest: packed.Digest, Format: packed.Format, FormatVersion: packed.FormatVersion},
			limits: ExtractLimits{MaxUnpackedSize: 1 << 20, MaxFileCount: 10},
			check: func(t *testing.T, target string) {
				info, statErr := os.Stat(filepath.Join(target, "common.sh"))
				require.NoError(t, statErr)
				require.Equal(t, PermReadOnly, info.Mode().Perm())
			},
		},
		{
			name:      "不支持的格式拒绝解压",
			meta:      Metadata{Digest: packed.Digest, Format: "unsupported", FormatVersion: packed.FormatVersion},
			limits:    ExtractLimits{MaxUnpackedSize: 1 << 20, MaxFileCount: 10},
			wantError: "不支持的制品格式",
		},
		{
			name:      "非法解压限制参数拒绝执行",
			meta:      Metadata{Digest: packed.Digest, Format: packed.Format, FormatVersion: packed.FormatVersion},
			limits:    ExtractLimits{MaxUnpackedSize: 0, MaxFileCount: 10},
			wantError: "制品解压限制非法",
		},
		{
			name:      "解压大小超出限制拒绝执行",
			meta:      Metadata{Digest: packed.Digest, Format: packed.Format, FormatVersion: packed.FormatVersion},
			limits:    ExtractLimits{MaxUnpackedSize: 1, MaxFileCount: 10},
			wantError: "制品解压大小超出限制",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			target := t.TempDir()
			extractErr := codec.Extract(packed.Path, target, tc.meta, tc.limits)
			if tc.wantError != "" {
				require.ErrorContains(t, extractErr, tc.wantError)
				return
			}
			require.NoError(t, extractErr)
			if tc.check != nil {
				tc.check(t, target)
			}
		})
	}
}

func readArtifact(t *testing.T, filePath string) ([]string, Manifest) {
	t.Helper()
	file, err := os.Open(filePath)
	require.NoError(t, err)
	defer file.Close()
	decoder, err := zstd.NewReader(file)
	require.NoError(t, err)
	defer decoder.Close()

	reader := tar.NewReader(decoder)
	var entries []string
	var manifest Manifest
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return entries, manifest
		}
		require.NoError(t, err)
		entries = append(entries, header.Name)
		if header.Name == ManifestPath {
			content, readErr := io.ReadAll(reader)
			require.NoError(t, readErr)
			require.NoError(t, json.Unmarshal(content, &manifest))
		}
	}
}
