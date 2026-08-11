package grpc

import (
	"context"

	runnerv1 "github.com/Duke1616/etask/api/proto/gen/etask/runner/v1"
	"github.com/Duke1616/etask/internal/domain"
	runnerSvc "github.com/Duke1616/etask/internal/service/runner"
	"github.com/ecodeclub/ekit/slice"
	"github.com/gotomicro/ego/core/elog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RunnerServer 对外提供执行单元查询能力。
type RunnerServer struct {
	runnerv1.UnimplementedRunnerServiceServer
	svc    runnerSvc.Service
	logger *elog.Component
}

// NewRunnerServer 创建执行单元 gRPC 服务端。
func NewRunnerServer(svc runnerSvc.Service) *RunnerServer {
	return &RunnerServer{
		svc:    svc,
		logger: elog.DefaultLogger.With(elog.FieldComponentName("grpc.RunnerServer")),
	}
}

// FindRunnerByID 根据执行单元 ID 获取执行单元。
func (s *RunnerServer) FindRunnerByID(ctx context.Context, req *runnerv1.FindRunnerByIDRequest) (*runnerv1.FindRunnerByIDResponse, error) {
	r, err := s.svc.FindByID(ctx, req.GetId())
	if err != nil {
		s.logger.Error("获取执行单元失败",
			elog.Int64("runnerID", req.GetId()),
			elog.FieldErr(err))
		return nil, status.Errorf(codes.NotFound, "runner not found: %v", err)
	}
	return &runnerv1.FindRunnerByIDResponse{Runner: s.toProto(r)}, nil
}

// ListRunnersByCodebookID 获取脚本文件下可用的全部执行单元。
func (s *RunnerServer) ListRunnersByCodebookID(ctx context.Context,
	req *runnerv1.ListRunnersByCodebookIDRequest) (*runnerv1.ListRunnersByCodebookIDResponse, error) {
	runners, err := s.svc.ListByCodebookID(ctx, req.GetCodebookId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list runners by codebook: %v", err)
	}
	return &runnerv1.ListRunnersByCodebookIDResponse{
		Runners: slice.Map(runners, func(_ int, spec domain.RunnerExecutionSpec) *runnerv1.Runner {
			result := s.toProto(spec.Runner)
			result.Variables = toProtoVariables(spec.Variables)
			return result
		}),
	}, nil
}

func (s *RunnerServer) toProto(r domain.Runner) *runnerv1.Runner {
	return &runnerv1.Runner{
		Id:                r.ID,
		Name:              r.Name,
		CodebookId:        r.CodebookID,
		ProgramKind:       string(r.ProgramKind),
		ParameterDefaults: runnerSvc.ParameterDefaultsBytes(r.ParameterDefaults),
		CodebookSecret:    r.CodebookSecret,
		Kind:              r.Kind.String(),
		Target:            r.Target,
		Handler:           r.Handler,
		Tags:              r.Tags,
		Action:            uint32(r.Action),
		Desc:              r.Desc,
		Ctime:             r.CTime,
		Utime:             r.UTime,
		Variables:         toProtoVariables(r.Variables),
	}
}

func toProtoVariables(variables []domain.RunnerVariable) []*runnerv1.Variable {
	return slice.Map(variables, func(_ int, src domain.RunnerVariable) *runnerv1.Variable {
		return &runnerv1.Variable{
			Key:    src.Key,
			Value:  src.Value,
			Secret: src.Secret,
		}
	})
}
