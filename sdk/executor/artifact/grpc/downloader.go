// Package grpc adapts the etask gRPC artifact service to the transport-neutral SDK contract.
package grpc

import (
	"context"
	"errors"
	"fmt"
	"io"

	artifactv1 "github.com/Duke1616/etask/api/proto/gen/etask/artifact/v1"
	"github.com/Duke1616/etask/sdk/executor/artifact"
)

type downloader struct {
	client artifactv1.ArtifactServiceClient
}

// NewDownloader adapts an ArtifactService client to an artifact.Downloader.
func NewDownloader(client artifactv1.ArtifactServiceClient) artifact.Downloader {
	return downloader{client: client}
}

func (d downloader) Download(ctx context.Context, ref artifact.Ref, target io.Writer) error {
	if d.client == nil {
		return fmt.Errorf("制品下载客户端尚未初始化")
	}
	stream, err := d.client.DownloadArtifact(ctx, &artifactv1.DownloadArtifactRequest{
		ReleaseId: ref.ReleaseID,
		Digest:    ref.Digest,
	})
	if err != nil {
		return fmt.Errorf("请求下载制品失败: %w", err)
	}
	for {
		chunk, receiveErr := stream.Recv()
		switch {
		case receiveErr == nil:
			if _, err = target.Write(chunk.GetData()); err != nil {
				return err
			}
		case errors.Is(receiveErr, io.EOF):
			return nil
		default:
			return fmt.Errorf("接收制品数据失败: %w", receiveErr)
		}
	}
}

var _ artifact.Downloader = downloader{}
