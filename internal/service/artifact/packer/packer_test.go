package packer

import (
	"archive/tar"
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
)

func TestPackerProducesDeterministicArtifact(t *testing.T) {
	p := New(t.TempDir())
	files := []domain.ArtifactFile{
		{Path: "python/util.py", Code: "VALUE = 2\n"},
		{Path: "common.sh", Code: "echo common\n"},
	}

	first, err := p.Pack(files)
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(first.Path) })
	second, err := p.Pack([]domain.ArtifactFile{files[1], files[0]})
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(second.Path) })

	require.Equal(t, first.Digest, second.Digest)
	require.Equal(t, first.BlobChecksum, second.BlobChecksum)
	require.Equal(t, first.Size, second.Size)

	entries, manifest := readArtifact(t, first.Path)
	require.Equal(t, []string{ManifestPath, "common.sh", "python/util.py"}, entries)
	require.Equal(t, first.Digest, manifest.Digest)
	require.Len(t, manifest.Files, 2)
	require.Equal(t, "common.sh", manifest.Files[0].Path)
	require.Equal(t, "python/util.py", manifest.Files[1].Path)
}

func TestPackerRejectsInvalidFiles(t *testing.T) {
	p := New(t.TempDir())
	tests := []struct {
		name  string
		files []domain.ArtifactFile
		text  string
	}{
		{name: "empty", text: "没有可发布的文件"},
		{name: "path traversal", files: []domain.ArtifactFile{{Path: "../secret", Code: "x"}}, text: "超出根目录"},
		{name: "non canonical path", files: []domain.ArtifactFile{{Path: "lib/../secret", Code: "x"}}, text: "规范相对路径"},
		{name: "reserved path", files: []domain.ArtifactFile{{Path: ".etask/manifest.json", Code: "x"}}, text: "保留目录"},
		{name: "duplicate", files: []domain.ArtifactFile{{Path: "a.py", Code: "x"}, {Path: "a.py", Code: "y"}}, text: "重复路径"},
		{name: "hash mismatch", files: []domain.ArtifactFile{{Path: "a.py", Code: "x", Hash: "invalid"}}, text: "校验和与版本记录不一致"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := p.Pack(tt.files)
			require.ErrorContains(t, err, tt.text)
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
