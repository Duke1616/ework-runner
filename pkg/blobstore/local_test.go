package blobstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalStoreLifecycle(t *testing.T) {
	store, err := NewLocal(t.TempDir())
	require.NoError(t, err)

	content := "print('artifact')\n"
	sum := sha256.Sum256([]byte(content))
	checksum := hex.EncodeToString(sum[:])
	key := "artifacts/release.tar.zst"

	err = store.Put(context.Background(), key, strings.NewReader(content), PutOptions{
		Size: int64(len(content)), Checksum: checksum,
	})
	require.NoError(t, err)

	reader, err := store.Open(context.Background(), key)
	require.NoError(t, err)
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	require.Equal(t, content, string(data))
	require.NoError(t, store.Delete(context.Background(), key))
	_, err = store.Open(context.Background(), key)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestLocalStoreRejectsInvalidInput(t *testing.T) {
	store, err := NewLocal(t.TempDir())
	require.NoError(t, err)

	err = store.Put(context.Background(), "../outside", strings.NewReader("x"), PutOptions{Size: 1})
	require.Error(t, err)
	err = store.Put(context.Background(), "artifact", strings.NewReader("x"), PutOptions{Size: 2})
	require.ErrorContains(t, err, "Blob 大小不一致")
	err = store.Put(context.Background(), "artifact", strings.NewReader("x"), PutOptions{
		Size: 1, Checksum: strings.Repeat("0", 64),
	})
	require.ErrorContains(t, err, "Blob 校验和不一致")
}
