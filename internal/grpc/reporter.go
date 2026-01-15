package grpc

import (
	"context"
	"fmt"

	reporterv1 "github.com/Duke1616/ework-runner/api/proto/gen/reporter/v1"
	"github.com/Duke1616/ework-runner/internal/domain"
	"github.com/Duke1616/ework-runner/internal/service/task"
	"github.com/ecodeclub/ekit/slice"
	"github.com/gotomicro/ego/core/elog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ReporterServer ReporterService gRPC服务实现
type ReporterServer struct {
	reporterv1.UnimplementedReporterServiceServer
	execSvc task.ExecutionService
	logger  *elog.Component
}

// NewReporterServer 创建 ReporterServer 实例
func NewReporterServer(
	execSvc task.ExecutionService,
) *ReporterServer {
	return &ReporterServer{
		execSvc: execSvc,
		logger:  elog.DefaultLogger.With(elog.FieldComponentName("scheduler.grpc.ReporterServer")),
	}
}

// Report 单个上报进度
func (s *ReporterServer) Report(ctx context.Context, req *reporterv1.ReportRequest) (*reporterv1.ReportResponse, error) {
	state := req.ExecutionState
	if state == nil {
		s.logger.Warn("收到空的执行状态上报请求")
		return &reporterv1.ReportResponse{}, nil
	}

	s.logger.Info("收到执行状态上报请求",
		elog.Int64("executionId", state.Id),
		elog.String("taskName", state.TaskName),
		elog.String("status", state.Status.String()),
		elog.String("requestReschedule", fmt.Sprintf("%v", state.RequestReschedule)))

	// 调用业务处理方法
	err := s.handleReports(ctx, s.toDomainReports([]*reporterv1.ReportRequest{req}))
	if err != nil {
		s.logger.Error("处理执行状态上报失败",
			elog.Int64("executionId", state.Id),
			elog.String("taskName", state.TaskName),
			elog.FieldErr(err))
		return nil, status.Error(codes.Internal, "处理失败")
	}

	s.logger.Debug("执行状态上报处理成功",
		elog.Int64("executionId", state.Id))
	return &reporterv1.ReportResponse{}, nil
}

// toDomainReports 将protobuf ExecutionState转换为domain.Report
func (s *ReporterServer) toDomainReports(reqs []*reporterv1.ReportRequest) []*domain.Report {
	return slice.Map(reqs, func(_ int, src *reporterv1.ReportRequest) *domain.Report {
		return &domain.Report{
			ExecutionState: domain.ExecutionStateFromProto(src.GetExecutionState()),
		}
	})
}

// handleReports 处理报告
func (s *ReporterServer) handleReports(ctx context.Context, reports []*domain.Report) error {
	s.logger.Debug("处理执行状态上报", elog.Int("count", len(reports)))
	return s.execSvc.HandleReports(ctx, reports)
}

// BatchReport 批量上报进度
func (s *ReporterServer) BatchReport(ctx context.Context, req *reporterv1.BatchReportRequest) (*reporterv1.BatchReportResponse, error) {
	if len(req.Reports) == 0 {
		s.logger.Debug("收到空的批量执行状态上报请求")
		return &reporterv1.BatchReportResponse{}, nil
	}

	s.logger.Info("收到批量执行状态上报请求", elog.Int("count", len(req.Reports)))

	err := s.handleReports(ctx, s.toDomainReports(req.GetReports()))
	if err != nil {
		s.logger.Error("处理批量执行状态上报失败",
			elog.Int("count", len(req.Reports)),
			elog.FieldErr(err))
		return nil, status.Error(codes.Internal, "处理失败")
	}

	s.logger.Debug("批量执行状态上报处理成功", elog.Int("count", len(req.Reports)))
	return &reporterv1.BatchReportResponse{}, nil
}
