package grpc

import (
	"errors"
	"io"

	artifactv1 "github.com/Duke1616/etask/api/proto/gen/etask/artifact/v1"
	artifactSvc "github.com/Duke1616/etask/internal/service/artifact"
	programSvc "github.com/Duke1616/etask/internal/service/program"
	"github.com/Duke1616/etask/pkg/blobstore"
	"github.com/gotomicro/ego/core/elog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const artifactChunkSize = 256 * 1024

type ArtifactServer struct {
	artifactv1.UnimplementedArtifactServiceServer
	svc     artifactSvc.Service
	program programSvc.Service
	logger  *elog.Component
}

func NewArtifactServer(svc artifactSvc.Service, program programSvc.Service) *ArtifactServer {
	return &ArtifactServer{
		svc:     svc,
		program: program,
		logger:  elog.DefaultLogger.With(elog.FieldComponentName("grpc.ArtifactServer")),
	}
}

func (s *ArtifactServer) DownloadArtifact(req *artifactv1.DownloadArtifactRequest,
	stream artifactv1.ArtifactService_DownloadArtifactServer) error {
	// Service 同时校验发布 ID 和摘要，gRPC 层只负责错误码映射与流式传输。
	reader, err := s.svc.Open(stream.Context(), req.GetReleaseId(), req.GetDigest())
	if err != nil {
		s.logger.Error("打开制品失败", elog.String("digest", req.GetDigest()), elog.FieldErr(err))
		if errors.Is(err, blobstore.ErrNotFound) {
			return status.Error(codes.NotFound, "制品不存在")
		}
		return status.Errorf(codes.Internal, "打开制品失败: %v", err)
	}
	return s.send(reader, stream, "制品")
}

func (s *ArtifactServer) DownloadProjectSource(req *artifactv1.DownloadProjectSourceRequest,
	stream artifactv1.ArtifactService_DownloadProjectSourceServer) error {
	reader, err := s.program.OpenSource(stream.Context(), req.GetSourceId(), req.GetDigest())
	if err != nil {
		s.logger.Error("打开项目源码失败", elog.String("digest", req.GetDigest()), elog.FieldErr(err))
		if errors.Is(err, blobstore.ErrNotFound) {
			return status.Error(codes.NotFound, "项目源码不存在")
		}
		return status.Errorf(codes.Internal, "打开项目源码失败: %v", err)
	}
	return s.send(reader, stream, "项目源码")
}

type artifactChunkStream interface {
	Send(*artifactv1.ArtifactChunk) error
}

func (s *ArtifactServer) send(reader io.ReadCloser, stream artifactChunkStream, name string) error {
	defer reader.Close()
	// 固定块大小限制单次内存占用；复制切片避免后续读取覆盖已发送内容。
	buffer := make([]byte, artifactChunkSize)
	for {
		n, readErr := reader.Read(buffer)
		if n > 0 {
			chunk := append([]byte(nil), buffer[:n]...)
			if sendErr := stream.Send(&artifactv1.ArtifactChunk{Data: chunk}); sendErr != nil {
				return status.Errorf(codes.Unavailable, "发送%s数据失败: %v", name, sendErr)
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return status.Errorf(codes.Internal, "读取%s数据失败: %v", name, readErr)
		}
	}
}
