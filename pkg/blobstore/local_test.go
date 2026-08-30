package blobstore

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalStore_Lifecycle(t *testing.T) {
	testCases := []struct {
		name     string
		key      string
		content  string
		opts     func(content string) PutOptions
	}{
		{
			name:    "写入读取并删除合法对象",
			key:     "artifacts/release.tar.zst",
			content: "print('artifact')\n",
			opts: func(content string) PutOptions {
				sum := sha256.Sum256([]byte(content))
				return PutOptions{
					Size:     int64(len(content)),
					Checksum: hex.EncodeToString(sum[:]),
				}
			},
		},
		{
			name:    "多级嵌套路径对象",
			key:     "tenants/10/projects/20/code.bin",
			content: "binary-payload-data",
			opts: func(content string) PutOptions {
				sum := sha256.Sum256([]byte(content))
				return PutOptions{
					Size:     int64(len(content)),
					Checksum: hex.EncodeToString(sum[:]),
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			store, err := NewLocal(t.TempDir())
			require.NoError(t, err)
			ctx := t.Context()

			// 1. 写入前验证不存在
			exists, err := store.Exists(ctx, tc.key)
			require.NoError(t, err)
			require.False(t, exists)

			// 2. 写入对象
			opts := tc.opts(tc.content)
			err = store.Put(ctx, tc.key, strings.NewReader(tc.content), opts)
			require.NoError(t, err)

			// 3. 写入后验证存在
			exists, err = store.Exists(ctx, tc.key)
			require.NoError(t, err)
			require.True(t, exists)

			// 4. 打开并读取内容校验
			reader, err := store.Open(ctx, tc.key)
			require.NoError(t, err)
			data, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.NoError(t, reader.Close())
			require.Equal(t, tc.content, string(data))

			// 5. 删除对象
			require.NoError(t, store.Delete(ctx, tc.key))

			// 6. 删除后验证不存在与 NotFound 错误
			exists, err = store.Exists(ctx, tc.key)
			require.NoError(t, err)
			require.False(t, exists)

			_, err = store.Open(ctx, tc.key)
			require.ErrorIs(t, err, ErrNotFound)
		})
	}
}

func TestLocalStore_Validation(t *testing.T) {
	testCases := []struct {
		name      string
		key       string
		content   string
		options   PutOptions
		errTarget error
		errMsg    string
	}{
		{
			name:      "拒绝父级目录穿越",
			key:       "../outside",
			content:   "x",
			options:   PutOptions{Size: 1},
			errTarget: ErrInvalidKey,
		},
		{
			name:      "拒绝绝对路径",
			key:       "/absolute/path",
			content:   "x",
			options:   PutOptions{Size: 1},
			errTarget: ErrInvalidKey,
		},
		{
			name:      "拒绝反斜杠路径",
			key:       "dir\\sub",
			content:   "x",
			options:   PutOptions{Size: 1},
			errTarget: ErrInvalidKey,
		},
		{
			name:      "拒绝大小不一致",
			key:       "valid/file",
			content:   "x",
			options:   PutOptions{Size: 2},
			errMsg:    "Blob 大小不一致",
		},
		{
			name:      "拒绝校验和不一致",
			key:       "valid/file",
			content:   "x",
			options:   PutOptions{Size: 1, Checksum: strings.Repeat("0", 64)},
			errMsg:    "Blob 校验和不一致",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			store, err := NewLocal(t.TempDir())
			require.NoError(t, err)

			err = store.Put(t.Context(), tc.key, strings.NewReader(tc.content), tc.options)
			require.Error(t, err)
			if tc.errTarget != nil {
				require.ErrorIs(t, err, tc.errTarget)
			}
			if tc.errMsg != "" {
				require.ErrorContains(t, err, tc.errMsg)
			}
		})
	}
}
