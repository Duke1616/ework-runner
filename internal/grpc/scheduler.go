package grpc

import (
	"context"
	"errors"

	schedulerv1 "github.com/Duke1616/etask/api/proto/gen/etask/scheduler/v1"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/service/submission"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SchedulerServer 将外部执行协议适配到提交应用服务。
type SchedulerServer struct {
	schedulerv1.UnimplementedSchedulerServiceServer
	svc submission.Service
}

// NewSchedulerServer 创建外部工作流执行 gRPC 服务。
func NewSchedulerServer(svc submission.Service) *SchedulerServer {
	return &SchedulerServer{svc: svc}
}

// RunRunner 幂等提交一次 Runner 正式执行。
func (s *SchedulerServer) RunRunner(ctx context.Context,
	req *schedulerv1.RunRunnerRequest) (*schedulerv1.RunRunnerResponse, error) {
	result, err := s.svc.RunRunner(ctx, submission.RunRunnerCommand{
		RequestID:   req.GetRequestId(),
		RunnerID:    req.GetRunnerId(),
		ProgramKind: toDomainProgramKind(req.GetProgramKind()),
		Params:      req.GetParams(),
		Variables:   req.GetVariables(),
	})
	if err != nil {
		switch {
		case errors.Is(err, submission.ErrInvalidCommand):
			return nil, status.Errorf(codes.InvalidArgument, "提交工作流执行失败: %v", err)
		case errors.Is(err, submission.ErrRejected):
			return nil, status.Errorf(codes.FailedPrecondition, "提交工作流执行失败: %v", err)
		default:
			return nil, status.Errorf(codes.Internal, "提交工作流执行失败: %v", err)
		}
	}
	return &schedulerv1.RunRunnerResponse{
		ExecutionId: result.Execution.ID,
		Created:     result.Created,
	}, nil
}

func toDomainProgramKind(kind schedulerv1.ProgramKind) domain.ProgramKind {
	switch kind {
	case schedulerv1.ProgramKind_PROGRAM_KIND_UNSPECIFIED:
		return domain.ProgramInline
	case schedulerv1.ProgramKind_PROGRAM_KIND_INLINE:
		return domain.ProgramInline
	case schedulerv1.ProgramKind_PROGRAM_KIND_PROJECT:
		return domain.ProgramProject
	default:
		return domain.ProgramKind(kind.String())
	}
}

func (s *SchedulerServer) TerminateExecution(ctx context.Context,
	req *schedulerv1.TerminateExecutionRequest) (*schedulerv1.TerminateExecutionResponse, error) {
	err := s.svc.TerminateExecution(ctx, submission.TerminateExecutionCommand{
		ExecutionID: req.GetExecutionId(), RequestID: req.GetRequestId(), Reason: req.GetReason(),
	})
	if err != nil {
		switch {
		case errors.Is(err, submission.ErrInvalidCommand):
			return nil, status.Errorf(codes.InvalidArgument, "终止工作流执行失败: %v", err)
		case errors.Is(err, submission.ErrRejected):
			return nil, status.Errorf(codes.FailedPrecondition, "终止工作流执行失败: %v", err)
		default:
			return nil, status.Errorf(codes.Internal, "终止工作流执行失败: %v", err)
		}
	}
	return &schedulerv1.TerminateExecutionResponse{Terminated: true}, nil
}
