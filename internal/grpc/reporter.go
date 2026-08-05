package grpc

import (
	"context"
	"errors"
	"fmt"

	reporterv1 "github.com/Duke1616/etask/api/proto/gen/etask/reporter/v1"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/gotomicro/ego/core/elog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

//go:generate go tool mockgen -source=./reporter.go -package=grpcmocks -destination=./mocks/reporter.mock.go -typed

// ExecutionReportHandler 定义执行节点上报所需的最小应用层能力。
type ExecutionReportHandler interface {
	// AppendExecutionLogs 持久化一批增量日志并广播给 SSE 订阅者。
	AppendExecutionLogs(ctx context.Context, executionID, taskID int64, logs []string) error
	// UpdateState 将执行节点上报的状态交给统一状态机处理。
	UpdateState(ctx context.Context, state domain.ExecutionState) error
}

// ReporterServer ReporterService gRPC服务实现
type ReporterServer struct {
	reporterv1.UnimplementedReporterServiceServer
	execSvc ExecutionReportHandler
	logger  *elog.Component
}

// NewReporterServer 创建 ReporterServer 实例
func NewReporterServer(
	execSvc ExecutionReportHandler,
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
	err := s.handleReport(ctx, req)
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

func (s *ReporterServer) handleReport(ctx context.Context, req *reporterv1.ReportRequest) error {
	if req.GetExecutionState() == nil {
		return fmt.Errorf("执行状态不能为空")
	}
	state := domain.ExecutionStateFromProto(req.GetExecutionState())
	if err := s.execSvc.AppendExecutionLogs(ctx, state.ID, state.TaskID, req.GetLogChunks()); err != nil {
		return err
	}
	if req.GetLogOnly() {
		return nil
	}
	return s.execSvc.UpdateState(ctx, state)
}

// BatchReport 批量上报进度
func (s *ReporterServer) BatchReport(ctx context.Context, req *reporterv1.BatchReportRequest) (*reporterv1.BatchReportResponse, error) {
	if len(req.Reports) == 0 {
		s.logger.Debug("收到空的批量执行状态上报请求")
		return &reporterv1.BatchReportResponse{}, nil
	}

	s.logger.Info("收到批量执行状态上报请求", elog.Int("count", len(req.Reports)))

	var err error
	for _, report := range req.GetReports() {
		err = errors.Join(err, s.handleReport(ctx, report))
	}
	if err != nil {
		s.logger.Error("处理批量执行状态上报失败",
			elog.Int("count", len(req.Reports)),
			elog.FieldErr(err))
		return nil, status.Error(codes.Internal, "处理失败")
	}

	s.logger.Debug("批量执行状态上报处理成功", elog.Int("count", len(req.Reports)))
	return &reporterv1.BatchReportResponse{}, nil
}
