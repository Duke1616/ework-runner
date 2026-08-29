package artifact

// 缓存测试覆盖下载、校验和缓存替换。

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	artifactv1 "github.com/Duke1616/etask/api/proto/gen/etask/artifact/v1"
	artifactarchive "github.com/Duke1616/etask/internal/artifact/archive"
	executorartifact "github.com/Duke1616/etask/sdk/executor/artifact"
	artifactgrpc "github.com/Duke1616/etask/sdk/executor/artifact/grpc"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestArtifactCacheEnsure(t *testing.T) {
	type state struct {
		cache       *artifactCache
		archive     []byte
		ref         executorartifact.Ref
		layer       layerRef
		downloader  executorartifact.Downloader
		closeServer func()
	}
	testCases := []struct {
		name       string
		path       string
		content    string
		before     func(t *testing.T, state *state)
		after      func(t *testing.T, state *state)
		wantError  string
		assertions func(t *testing.T, state *state, root string)
	}{
		{
			name: "替换无效缓存并复用完成层", path: "lib/common.py", content: "VALUE = 1\n",
			before: func(t *testing.T, current *state) {
				ref, err := parseArtifactLayerRef(current.ref)
				require.NoError(t, err)
				target := current.cache.layout.layerDir(ref)
				require.NoError(t, os.MkdirAll(target, 0o750))
				require.NoError(t, os.WriteFile(filepath.Join(target, ".ready"), []byte("broken"), 0o440))
			},
			assertions: func(t *testing.T, current *state, root string) {
				content, err := os.ReadFile(filepath.Join(root, "lib", "common.py"))
				require.NoError(t, err)
				require.Equal(t, "VALUE = 1\n", string(content))
				current.closeServer()
				cached, err := current.cache.Ensure(t.Context(), current.downloader, current.layer)
				require.NoError(t, err)
				require.Equal(t, root, cached)
				current.closeServer = nil
			},
		},
		{
			name: "拒绝越界路径", path: "../outside", content: "secret", wantError: "超出缓存目录",
			after: func(t *testing.T, current *state) {
				_, err := os.Stat(filepath.Join(current.cache.cfg.Dir, "outside"))
				require.ErrorIs(t, err, os.ErrNotExist)
			},
		},
		{
			name: "拒绝清单摘要不一致", path: "lib/common.py", content: "VALUE = 1\n", wantError: "清单与制品引用不一致",
			before: func(_ *testing.T, current *state) { current.ref.Digest = strings.Repeat("a", 64) },
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			current := &state{cache: newArtifactCache(Config{Dir: t.TempDir()})}
			current.archive, current.ref = buildTestRef(t, tc.path, tc.content)
			client, closeServer := newArtifactClient(t, current.archive)
			current.downloader, current.closeServer = artifactgrpc.NewDownloader(client), closeServer
			defer func() {
				if current.closeServer != nil {
					current.closeServer()
				}
			}()
			if tc.before != nil {
				tc.before(t, current)
			}
			if tc.after != nil {
				defer tc.after(t, current)
			}
			layer, err := parseArtifactLayerRef(current.ref)
			require.NoError(t, err)
			current.layer = layer
			root, err := current.cache.Ensure(t.Context(), current.downloader, current.layer)
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				return
			}
			require.NoError(t, err)
			tc.assertions(t, current, root)
		})
	}
}

func TestArtifactCacheEnsureRejectsMissingClient(t *testing.T) {
	ref, err := parseArtifactLayerRef(validRef("ops_common"))
	require.NoError(t, err)
	_, err = newArtifactCache(Config{Dir: t.TempDir()}).Ensure(t.Context(), nil, ref)
	require.ErrorContains(t, err, "客户端尚未初始化")
}

func TestArtifactCachePrunesOldLayers(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Chmod(root, 0o755))
	cache := newArtifactCache(Config{Dir: root, MaxCacheSize: 9})
	layersDir := filepath.Join(root, "layers")
	oldLayer := filepath.Join(layersDir, "old")
	newLayer := filepath.Join(layersDir, "new")
	for _, layer := range []string{oldLayer, newLayer} {
		require.NoError(t, os.MkdirAll(layer, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(layer, ".ready"), []byte("ready"), 0o440))
		require.NoError(t, os.WriteFile(filepath.Join(layer, "data"), []byte("data"), 0o440))
	}
	oldTime := time.Now().Add(-time.Hour)
	require.NoError(t, os.Chtimes(filepath.Join(oldLayer, ".ready"), oldTime, oldTime))

	require.NoError(t, cache.Prune())
	rootInfo, err := os.Stat(root)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o750), rootInfo.Mode().Perm())
	_, err = os.Stat(oldLayer)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(newLayer)
	require.NoError(t, err)
}

func TestParseLayerRef(t *testing.T) {
	valid := executorartifact.Ref{
		ReleaseID: 1,
		Digest:    strings.Repeat("a", 64), BlobChecksum: strings.Repeat("b", 64), Size: 1,
		Format: artifactarchive.Format, FormatVersion: artifactarchive.FormatVersion,
	}
	invalidChecksum := valid
	invalidChecksum.BlobChecksum = "invalid"
	invalidFormat := valid
	invalidFormat.FormatVersion++
	testCases := []struct {
		name      string
		ref       executorartifact.Ref
		wantError string
	}{
		{name: "空引用", wantError: "发布 ID 非法"},
		{name: "合法引用", ref: valid},
		{name: "非法校验和", ref: invalidChecksum, wantError: "校验和非法"},
		{name: "不支持格式", ref: invalidFormat, wantError: "不支持的制品格式"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseArtifactLayerRef(tc.ref)
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestArtifactLayerKeySeparatesDifferentBlobs(t *testing.T) {
	first := validRef("")
	second := first
	second.BlobChecksum = strings.Repeat("c", 64)
	firstRef, err := parseArtifactLayerRef(first)
	require.NoError(t, err)
	secondRef, err := parseArtifactLayerRef(second)
	require.NoError(t, err)

	require.NotEqual(t, firstRef.cacheKey(), secondRef.cacheKey())
}

type artifactTestServer struct {
	artifactv1.UnimplementedArtifactServiceServer
	data []byte
}

type artifactClientStub struct {
	artifactv1.ArtifactServiceClient
}

func (s artifactTestServer) DownloadArtifact(_ *artifactv1.DownloadArtifactRequest,
	stream artifactv1.ArtifactService_DownloadArtifactServer) error {
	return stream.Send(&artifactv1.ArtifactChunk{Data: s.data})
}

func (s artifactTestServer) DownloadProjectSource(_ *artifactv1.DownloadProjectSourceRequest,
	stream artifactv1.ArtifactService_DownloadProjectSourceServer) error {
	return stream.Send(&artifactv1.ArtifactChunk{Data: s.data})
}

func newArtifactClient(t *testing.T, data []byte) (artifactv1.ArtifactServiceClient, func()) {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	artifactv1.RegisterArtifactServiceServer(server, artifactTestServer{data: data})
	go func() { _ = server.Serve(listener) }()
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	return artifactv1.NewArtifactServiceClient(conn), func() {
		_ = conn.Close()
		server.Stop()
		_ = listener.Close()
	}
}

func buildTestArtifact(t *testing.T, name, content string) ([]byte, string) {
	t.Helper()
	fileSum := sha256.Sum256([]byte(content))
	manifest := artifactarchive.Manifest{
		FormatVersion: artifactarchive.FormatVersion,
		Files: []artifactarchive.ManifestFile{{
			Path: name, Hash: hex.EncodeToString(fileSum[:]), Size: int64(len(content)),
		}},
	}
	identity, err := json.Marshal(manifest)
	require.NoError(t, err)
	digestBytes := sha256.Sum256(identity)
	manifest.Digest = hex.EncodeToString(digestBytes[:])
	manifestData, err := json.Marshal(manifest)
	require.NoError(t, err)

	var buffer bytes.Buffer
	encoder, err := zstd.NewWriter(&buffer)
	require.NoError(t, err)
	w := tar.NewWriter(encoder)
	require.NoError(t, w.WriteHeader(&tar.Header{
		Name: ".etask/manifest.json", Mode: 0o444, Size: int64(len(manifestData)), Typeflag: tar.TypeReg,
	}))
	_, err = w.Write(manifestData)
	require.NoError(t, err)
	require.NoError(t, w.WriteHeader(&tar.Header{Name: name, Mode: 0o444, Size: int64(len(content)), Typeflag: tar.TypeReg}))
	_, err = w.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	require.NoError(t, encoder.Close())
	return buffer.Bytes(), manifest.Digest
}

func buildTestRef(t *testing.T, name, content string) ([]byte, executorartifact.Ref) {
	archive, digest := buildTestArtifact(t, name, content)
	checksum := sha256.Sum256(archive)
	return archive, executorartifact.Ref{
		ReleaseID: 1, Digest: digest,
		BlobChecksum: hex.EncodeToString(checksum[:]), Size: int64(len(archive)),
		Format: artifactarchive.Format, FormatVersion: artifactarchive.FormatVersion,
	}
}

type countingDownloader struct {
	executorartifact.Downloader
	calls atomic.Int64
	delay time.Duration
}

func (d *countingDownloader) DownloadArtifact(ctx context.Context, ref executorartifact.Ref, target io.Writer) error {
	d.calls.Add(1)
	if d.delay > 0 {
		time.Sleep(d.delay)
	}
	return d.Downloader.DownloadArtifact(ctx, ref, target)
}

func TestArtifactCacheConcurrency(t *testing.T) {
	testCases := []struct {
		name string
		run  func(t *testing.T, cache *artifactCache, dl *countingDownloader, layer layerRef)
	}{
		{
			name: "SingleFlight 并发合并确保仅下载一次",
			run: func(t *testing.T, cache *artifactCache, dl *countingDownloader, layer layerRef) {
				const concurrency = 10
				var wg sync.WaitGroup
				wg.Add(concurrency)

				roots := make([]string, concurrency)
				errs := make([]error, concurrency)

				for i := 0; i < concurrency; i++ {
					go func(idx int) {
						defer wg.Done()
						root, err := cache.Ensure(context.Background(), dl, layer)
						roots[idx] = root
						errs[idx] = err
					}(i)
				}
				wg.Wait()

				for i := 0; i < concurrency; i++ {
					require.NoError(t, errs[i])
					require.Equal(t, roots[0], roots[i])
					content, readErr := os.ReadFile(filepath.Join(roots[i], "concurrency.txt"))
					require.NoError(t, readErr)
					require.Equal(t, "concurrency data\n", string(content))
				}
				// 验证 SingleFlight 确实合并了并发调用，实际物理下载只发生了一次
				require.Equal(t, int64(1), dl.calls.Load())
			},
		},
		{
			name: "并发中单个调用方取消不影响其他调用方成功物化",
			run: func(t *testing.T, cache *artifactCache, dl *countingDownloader, layer layerRef) {
				cancelCtx, cancel := context.WithCancel(context.Background())
				defer cancel()

				var wg sync.WaitGroup
				wg.Add(2)

				var err1, err2 error
				var root2 string

				// 请求者 1：中途取消
				go func() {
					defer wg.Done()
					time.Sleep(10 * time.Millisecond)
					cancel()
					_, err1 = cache.Ensure(cancelCtx, dl, layer)
				}()

				// 请求者 2：使用正常未取消的 context
				go func() {
					defer wg.Done()
					root2, err2 = cache.Ensure(context.Background(), dl, layer)
				}()

				wg.Wait()

				// 请求者 1 收到 context.Canceled，但后台任务脱钩执行不受影响
				require.ErrorIs(t, err1, context.Canceled)
				// 请求者 2 正常拿到解压目录
				require.NoError(t, err2)
				content, err := os.ReadFile(filepath.Join(root2, "concurrency.txt"))
				require.NoError(t, err)
				require.Equal(t, "concurrency data\n", string(content))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cache := newArtifactCache(Config{Dir: t.TempDir()})
			archive, ref := buildTestRef(t, "concurrency.txt", "concurrency data\n")
			client, closeServer := newArtifactClient(t, archive)
			defer closeServer()

			countingDl := &countingDownloader{
				Downloader: artifactgrpc.NewDownloader(client),
				delay:      40 * time.Millisecond,
			}
			layer, err := parseArtifactLayerRef(ref)
			require.NoError(t, err)

			tc.run(t, cache, countingDl, layer)
		})
	}
}

func TestArtifactCacheSelfHealing(t *testing.T) {
	testCases := []struct {
		name   string
		setup  func(t *testing.T, cache *artifactCache, layer layerRef)
		verify func(t *testing.T, cache *artifactCache, layer layerRef)
	}{
		{
			name: "清理无 ready 标记的半解压目录",
			setup: func(t *testing.T, cache *artifactCache, layer layerRef) {
				targetDir := cache.layout.layerDir(layer)
				require.NoError(t, os.MkdirAll(targetDir, artifactarchive.PermDir))
				require.NoError(t, os.WriteFile(filepath.Join(targetDir, "partial.tmp"), []byte("bad"), artifactarchive.PermReadOnly))
			},
			verify: func(t *testing.T, cache *artifactCache, layer layerRef) {
				targetDir := cache.layout.layerDir(layer)
				_, err := os.Stat(targetDir)
				require.ErrorIs(t, err, os.ErrNotExist)
			},
		},
		{
			name: "清理 layers 根目录下的非法普通文件",
			setup: func(t *testing.T, cache *artifactCache, layer layerRef) {
				require.NoError(t, os.MkdirAll(cache.layout.layersDir(), artifactarchive.PermDir))
				badFile := filepath.Join(cache.layout.layersDir(), "corrupt_layer_file")
				require.NoError(t, os.WriteFile(badFile, []byte("junk"), artifactarchive.PermReadOnly))
			},
			verify: func(t *testing.T, cache *artifactCache, layer layerRef) {
				badFile := filepath.Join(cache.layout.layersDir(), "corrupt_layer_file")
				_, err := os.Stat(badFile)
				require.ErrorIs(t, err, os.ErrNotExist)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cacheDir := t.TempDir()
			cache := newArtifactCache(Config{Dir: cacheDir})
			archive, ref := buildTestRef(t, "heal.py", "heal=True\n")
			client, closeServer := newArtifactClient(t, archive)
			defer closeServer()

			downloader := artifactgrpc.NewDownloader(client)
			layer, err := parseArtifactLayerRef(ref)
			require.NoError(t, err)

			tc.setup(t, cache, layer)

			require.NoError(t, cache.Prune())
			tc.verify(t, cache, layer)

			root, err := cache.Ensure(context.Background(), downloader, layer)
			require.NoError(t, err)
			content, err := os.ReadFile(filepath.Join(root, "heal.py"))
			require.NoError(t, err)
			require.Equal(t, "heal=True\n", string(content))
		})
	}
}
