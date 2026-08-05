package runtime

import (
	"context"

	artifactv1 "github.com/Duke1616/etask/api/proto/gen/etask/artifact/v1"
	executorv1 "github.com/Duke1616/etask/api/proto/gen/etask/executor/v1"
	reporterv1 "github.com/Duke1616/etask/api/proto/gen/etask/reporter/v1"
	"github.com/Duke1616/etask/pkg/tasklog"
	"github.com/Duke1616/etask/sdk/executor/artifact"
	"github.com/Duke1616/etask/sdk/executor/internal/task"
	"github.com/gotomicro/ego/core/elog"
)

type grpcLogSink struct {
	executionID int64
	reporter    reporterv1.ReporterServiceClient
}

func (s grpcLogSink) WriteBatch(ctx context.Context, logs []string) error {
	_, err := s.reporter.Report(ctx, &reporterv1.ReportRequest{
		ExecutionState: &executorv1.ExecutionState{Id: s.executionID},
		LogChunks:      logs,
		LogOnly:        true,
	})
	return err
}

func (e *Executor) newTaskLogger(ctx context.Context, executionID int64) task.Logger {
	if e.reporterClient == nil {
		return nil
	}
	return tasklog.New(ctx, grpcLogSink{executionID: executionID, reporter: e.reporterClient}, tasklog.Options{
		OnError: func(err error) {
			e.logger.Error("上报任务日志失败", elog.FieldErr(err))
		},
	})
}

func artifactRefs(values []*artifactv1.ArtifactRef) []artifact.Ref {
	refs := make([]artifact.Ref, 0, len(values))
	for _, value := range values {
		if value == nil {
			refs = append(refs, artifact.Ref{})
			continue
		}
		refs = append(refs, artifact.Ref{
			ReleaseID: value.GetReleaseId(), Digest: value.GetDigest(),
			BlobChecksum: value.GetBlobChecksum(), Size: value.GetSize(),
			Format: value.GetFormat(), FormatVersion: value.GetFormatVersion(),
			MountName: value.GetMountName(),
		})
	}
	return refs
}
