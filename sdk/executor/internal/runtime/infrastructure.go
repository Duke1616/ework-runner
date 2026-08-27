package runtime

import (
	"context"
	"errors"

	artifactv1 "github.com/Duke1616/etask/api/proto/gen/etask/artifact/v1"
	executorv1 "github.com/Duke1616/etask/api/proto/gen/etask/executor/v1"
	reporterv1 "github.com/Duke1616/etask/api/proto/gen/etask/reporter/v1"
	"github.com/Duke1616/etask/pkg/tasklog"
	"github.com/Duke1616/etask/sdk/executor/artifact"
	"github.com/Duke1616/etask/sdk/executor/internal/task"
	"github.com/gotomicro/ego/core/elog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

func (e *Executor) newExecutionLogger(ctx context.Context, executionID int64) task.ExecutionLogger {
	if e.reporterClient == nil {
		return nil
	}
	return tasklog.New(ctx, grpcLogSink{executionID: executionID, reporter: e.reporterClient}, tasklog.Options{
		OnError: func(err error) {
			if isConnectionClosingError(err) {
				e.logger.Debug("停止期间任务日志未上报", elog.FieldErr(err))
				return
			}
			e.logger.Error("上报任务日志失败", elog.FieldErr(err))
		},
	})
}

func isConnectionClosingError(err error) bool {
	return errors.Is(err, context.Canceled) || status.Code(err) == codes.Canceled
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

func programFromProto(value *executorv1.ProgramSource) (*task.Program, *artifact.SourceRef) {
	if value == nil {
		return nil, nil
	}
	result := &task.Program{}
	switch source := value.GetSource().(type) {
	case *executorv1.ProgramSource_Inline:
		result.Kind = task.ProgramInline
		result.Inline = &task.InlineProgram{Code: source.Inline.GetCode()}
	case *executorv1.ProgramSource_Project:
		result.Kind = task.ProgramProject
		ref := source.Project.GetSource()
		if ref == nil {
			return result, nil
		}
		result.Project = &task.ProjectProgram{
			EntryPoint: source.Project.GetEntryPoint(),
		}
		return result, &artifact.SourceRef{
			SourceID: ref.GetSourceId(), Digest: ref.GetDigest(), BlobChecksum: ref.GetBlobChecksum(),
			Size: ref.GetSize(), Format: ref.GetFormat(), FormatVersion: ref.GetFormatVersion(),
		}
	}
	return result, nil
}
