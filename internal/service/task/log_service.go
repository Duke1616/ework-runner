package task

import (
	"context"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository"
	"golang.org/x/sync/errgroup"
)

//go:generate go tool mockgen -source=./log_service.go -package=taskmocks -destination=./mocks/log_service.mock.go -typed

// LogService 任务日志服务接口
type LogService interface {
	// AddLog 添加任务日志
	AddLog(ctx context.Context, log domain.TaskExecutionLog) (domain.TaskExecutionLog, error)
	// BatchAddLogs 批量添加日志
	BatchAddLogs(ctx context.Context, logs []domain.TaskExecutionLog) error
	// GetLogs 获取执行任务日志和总数
	GetLogs(ctx context.Context, executionID int64, minID int64, limit int) ([]domain.TaskExecutionLog, int64, error)
}

type logService struct {
	repo repository.TaskExecutionLogRepository
}

func NewLogService(repo repository.TaskExecutionLogRepository) LogService {
	return &logService{
		repo: repo,
	}
}

func (s *logService) AddLog(ctx context.Context, log domain.TaskExecutionLog) (domain.TaskExecutionLog, error) {
	return s.repo.Create(ctx, log)
}

func (s *logService) BatchAddLogs(ctx context.Context, logs []domain.TaskExecutionLog) error {
	return s.repo.BatchCreate(ctx, logs)
}

func (s *logService) GetLogs(ctx context.Context, executionID int64, minID int64, limit int) ([]domain.TaskExecutionLog, int64, error) {
	var (
		logs  []domain.TaskExecutionLog
		total int64
	)
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		var err error
		logs, err = s.repo.GetLogsByExecutionID(ctx, executionID, minID, limit)
		return err
	})
	eg.Go(func() error {
		var err error
		total, err = s.repo.CountByExecutionID(ctx, executionID)
		return err
	})
	if err := eg.Wait(); err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}
