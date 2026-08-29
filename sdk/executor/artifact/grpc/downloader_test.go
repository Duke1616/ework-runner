package grpc

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	artifactv1 "github.com/Duke1616/etask/api/proto/gen/etask/artifact/v1"
	"github.com/Duke1616/etask/sdk/executor/artifact"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type mockChunkStream struct {
	grpc.ClientStream
	chunks [][]byte
	index  int
	err    error
}

func (m *mockChunkStream) Recv() (*artifactv1.ArtifactChunk, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.index >= len(m.chunks) {
		return nil, io.EOF
	}
	chunk := &artifactv1.ArtifactChunk{Data: m.chunks[m.index]}
	m.index++
	return chunk, nil
}

type mockArtifactServiceClient struct {
	artifactStream grpc.ServerStreamingClient[artifactv1.ArtifactChunk]
	artifactErr    error
	sourceStream   grpc.ServerStreamingClient[artifactv1.ArtifactChunk]
	sourceErr      error
}

func (m *mockArtifactServiceClient) DownloadArtifact(
	_ context.Context, _ *artifactv1.DownloadArtifactRequest, _ ...grpc.CallOption,
) (grpc.ServerStreamingClient[artifactv1.ArtifactChunk], error) {
	return m.artifactStream, m.artifactErr
}

func (m *mockArtifactServiceClient) DownloadProjectSource(
	_ context.Context, _ *artifactv1.DownloadProjectSourceRequest, _ ...grpc.CallOption,
) (grpc.ServerStreamingClient[artifactv1.ArtifactChunk], error) {
	return m.sourceStream, m.sourceErr
}

func TestDownloader_UninitializedClient(t *testing.T) {
	d := NewDownloader(nil)
	var buf bytes.Buffer

	err := d.DownloadArtifact(t.Context(), artifact.Ref{}, &buf)
	require.ErrorContains(t, err, "客户端尚未初始化")

	err = d.DownloadSource(t.Context(), artifact.SourceRef{}, &buf)
	require.ErrorContains(t, err, "客户端尚未初始化")
}

func TestDownloader_DownloadArtifact(t *testing.T) {
	testCases := []struct {
		name      string
		stream    *mockChunkStream
		callErr   error
		wantData  string
		wantErr   string
	}{
		{
			name: "成功分块流式写入并终止于 EOF",
			stream: &mockChunkStream{
				chunks: [][]byte{[]byte("chunk-1;"), []byte("chunk-2;")},
			},
			wantData: "chunk-1;chunk-2;",
		},
		{
			name:    "RPC 调用直接返回错误",
			callErr: errors.New("network unavailable"),
			wantErr: "请求下载制品失败",
		},
		{
			name: "流中途传输中断返回错误",
			stream: &mockChunkStream{
				err: errors.New("connection reset"),
			},
			wantErr: "接收制品数据失败",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := &mockArtifactServiceClient{
				artifactStream: tc.stream,
				artifactErr:    tc.callErr,
			}
			d := NewDownloader(client)
			var buf bytes.Buffer

			err := d.DownloadArtifact(t.Context(), artifact.Ref{ReleaseID: 101, Digest: "sha256:abc"}, &buf)
			if tc.wantErr != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantData, buf.String())
		})
	}
}

func TestDownloader_DownloadSource(t *testing.T) {
	testCases := []struct {
		name     string
		stream   *mockChunkStream
		callErr  error
		wantData string
		wantErr  string
	}{
		{
			name: "成功下载项目源码流",
			stream: &mockChunkStream{
				chunks: [][]byte{[]byte("source-tar-chunk")},
			},
			wantData: "source-tar-chunk",
		},
		{
			name:    "RPC 请求项目源码直接报错",
			callErr: errors.New("not found"),
			wantErr: "请求下载项目源码失败",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := &mockArtifactServiceClient{
				sourceStream: tc.stream,
				sourceErr:    tc.callErr,
			}
			d := NewDownloader(client)
			var buf bytes.Buffer

			err := d.DownloadSource(t.Context(), artifact.SourceRef{SourceID: 202, Digest: "sha256:source"}, &buf)
			if tc.wantErr != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantData, buf.String())
		})
	}
}
