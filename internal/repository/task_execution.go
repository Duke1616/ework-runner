package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository/dao"
	"github.com/Duke1616/etask/pkg/cryptox"
	"github.com/Duke1616/etask/pkg/sqlx"
	"gorm.io/gorm"
)

//go:generate go tool mockgen -source=./task_execution.go -package=repositorymocks -destination=./mocks/task_execution.mock.go -typed

type TaskExecutionRepository interface {
	// Create 创建任务执行实例
	Create(ctx context.Context, execution domain.TaskExecution) (domain.TaskExecution, error)
	// UpdateStatus 仅在当前状态符合预期时更新执行状态。
	UpdateStatus(ctx context.Context, id int64, expectedStatuses []domain.TaskExecutionStatus,
		status domain.TaskExecutionStatus) error
	// GetByID 根据ID获取执行实例
	GetByID(ctx context.Context, id int64) (domain.TaskExecution, error)
	// FindByRequestID 根据执行来源和幂等请求标识查询执行实例。
	FindByRequestID(ctx context.Context, source domain.TaskExecutionSource, requestID string) (domain.TaskExecution, bool, error)
	// FindRetryableExecutions 查找所有可以重试的执行记录
	// limit: 查询结果数量限制
	FindRetryableExecutions(ctx context.Context, limit int) ([]domain.TaskExecution, error)
	// UpdateRetryResult 仅在当前状态符合预期时更新重试结果。
	UpdateRetryResult(ctx context.Context, id, retryCount, nextRetryTime int64,
		expectedStatus, status domain.TaskExecutionStatus, progress int32, endTime int64,
		scheduleParams map[string]string, executorNodeID string) error
	// SetRunningState 设置任务为运行状态并更新进度
	SetRunningState(ctx context.Context, id int64, progress int32, executorNodeID string) error
	// UpdateRunningProgress 更新任务执行进度（仅在RUNNING状态下有效）
	UpdateRunningProgress(ctx context.Context, id int64, progress int32, executorNodeID string) error
	// UpdateScheduleResult 仅在当前状态符合预期时更新调度结果。
	// 返回 false 表示状态已被其他请求推进，当前请求没有写入。
	UpdateScheduleResult(ctx context.Context, id int64, expectedStatuses []domain.TaskExecutionStatus,
		status domain.TaskExecutionStatus, progress int32, endTime int64,
		scheduleParams map[string]string, executorNodeID string, taskResult string) (bool, error)
	// FindReschedulableExecutions 查找所有可以重调度的执行记录
	FindReschedulableExecutions(ctx context.Context, limit int) ([]domain.TaskExecution, error)

	// FindExecutionsByPlanExecID 查询计划执行下按任务 ID 索引的执行记录。
	FindExecutionsByPlanExecID(ctx context.Context, planExecID int64) (map[int64]domain.TaskExecution, error)
	// FindByTaskID 根据任务ID查找所有执行记录
	FindByTaskID(ctx context.Context, taskID int64) ([]domain.TaskExecution, error)
	// FindByTaskIDs 批量根据一批任务ID查找它们对应的所有执行记录
	FindByTaskIDs(ctx context.Context, taskIDs []int64) ([]domain.TaskExecution, error)
	// ListByTaskID 根据任务ID分页查找执行记录
	ListByTaskID(ctx context.Context, taskID int64, offset, limit int) ([]domain.TaskExecution, int64, error)
	// FindExecutionByTaskIDAndPlanExecID 根据任务ID和执行计划ID查找执行记录
	FindExecutionByTaskIDAndPlanExecID(ctx context.Context, taskID int64, planExecID int64) (domain.TaskExecution, error)
	// FindTimeoutExecutions 查找超时的执行记录
	FindTimeoutExecutions(ctx context.Context, limit int) ([]domain.TaskExecution, error)
	// ClaimPullTask 原子抢占一个当前节点支持的等待拉取任务。
	ClaimPullTask(ctx context.Context, serviceName, executorNodeID string,
		handlerNames []string) (domain.TaskExecution, error)
}

type taskExecutionRepository struct {
	dao    dao.TaskExecutionDAO
	crypto cryptox.Crypto
}

func NewTaskExecutionRepository(executionDAO dao.TaskExecutionDAO,
	crypto cryptox.Crypto) TaskExecutionRepository {
	return &taskExecutionRepository{
		dao: executionDAO, crypto: crypto,
	}
}

func (r *taskExecutionRepository) FindExecutionByTaskIDAndPlanExecID(ctx context.Context, taskID, planExecID int64) (domain.TaskExecution, error) {
	daoExec, err := r.dao.FindExecutionByTaskIDAndPlanExecID(ctx, taskID, planExecID)
	if err != nil {
		return domain.TaskExecution{}, err
	}
	return r.toDomain(daoExec)
}

func (r *taskExecutionRepository) FindByTaskID(ctx context.Context, taskID int64) ([]domain.TaskExecution, error) {
	daoExecutions, err := r.dao.FindByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return r.toDomains(daoExecutions)
}

func (r *taskExecutionRepository) FindByTaskIDs(ctx context.Context, taskIDs []int64) ([]domain.TaskExecution, error) {
	daoExecutions, err := r.dao.FindByTaskIDs(ctx, taskIDs)
	if err != nil {
		return nil, err
	}
	return r.toDomains(daoExecutions)
}

func (r *taskExecutionRepository) ListByTaskID(ctx context.Context, taskID int64, offset, limit int) ([]domain.TaskExecution, int64, error) {
	daoExecutions, err := r.dao.ListByTaskID(ctx, taskID, offset, limit)
	if err != nil {
		return nil, 0, err
	}

	total, err := r.dao.CountByTaskID(ctx, taskID)
	if err != nil {
		return nil, 0, err
	}

	executions, err := r.toDomains(daoExecutions)
	return executions, total, err
}

func (r *taskExecutionRepository) FindExecutionsByPlanExecID(ctx context.Context, planExecID int64) (map[int64]domain.TaskExecution, error) {
	daoExecutions, err := r.dao.FindExecutionByPlanID(ctx, planExecID)
	if err != nil {
		return nil, err
	}

	// 将DAO模型转换为领域模型
	result := make(map[int64]domain.TaskExecution)
	for taskID := range daoExecutions {
		daoExecution := daoExecutions[taskID]
		execution, convertErr := r.toDomain(daoExecution)
		if convertErr != nil {
			return nil, convertErr
		}
		result[taskID] = execution
	}

	return result, nil
}

func (r *taskExecutionRepository) Create(ctx context.Context, execution domain.TaskExecution) (domain.TaskExecution, error) {
	if err := execution.Route.Validate(); err != nil {
		return domain.TaskExecution{}, err
	}
	// 验证必填字段
	if execution.Source == "" {
		execution.Source = domain.TaskExecutionSourceTask
	}
	if execution.Task.ID == 0 && !execution.Source.AllowsEmptyTaskID() {
		return domain.TaskExecution{}, errors.New("任务 ID 不能为空")
	}

	// 自动继承 Task 的 TenantID（后台调度等无租户上下文的场景）
	if execution.TenantID == 0 {
		execution.TenantID = execution.Task.TenantID
	}

	entity, err := r.toEntity(execution)
	if err != nil {
		return domain.TaskExecution{}, err
	}
	var created dao.TaskExecution
	if execution.Source == domain.TaskExecutionSourceTask && len(execution.Task.PendingParamOverrides) > 0 {
		created, err = r.dao.CreateAndConsumeOverride(ctx, entity, execution.Task.ID)
	} else {
		created, err = r.dao.Create(ctx, entity)
	}
	if err != nil {
		return domain.TaskExecution{}, err
	}
	return r.toDomain(created)
}

func (r *taskExecutionRepository) UpdateStatus(ctx context.Context, id int64,
	expectedStatuses []domain.TaskExecutionStatus, status domain.TaskExecutionStatus) error {
	return r.dao.UpdateStatus(ctx, id, executionStatusStrings(expectedStatuses), status.String())
}

func (r *taskExecutionRepository) GetByID(ctx context.Context, id int64) (domain.TaskExecution, error) {
	daoExecution, err := r.dao.GetByID(ctx, id)
	if err != nil {
		return domain.TaskExecution{}, err
	}
	return r.toDomain(daoExecution)
}

func (r *taskExecutionRepository) FindByRequestID(ctx context.Context, source domain.TaskExecutionSource,
	requestID string) (domain.TaskExecution, bool, error) {
	execution, err := r.dao.FindByRequestID(ctx, source.String(), requestID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.TaskExecution{}, false, nil
	}
	if err != nil {
		return domain.TaskExecution{}, false, err
	}
	result, err := r.toDomain(execution)
	return result, true, err
}

func (r *taskExecutionRepository) FindRetryableExecutions(ctx context.Context, limit int) ([]domain.TaskExecution, error) {
	daoExecutions, err := r.dao.FindRetryableExecutions(ctx, limit)
	if err != nil {
		return nil, err
	}
	return r.toDomains(daoExecutions)
}

func (r *taskExecutionRepository) UpdateRetryResult(ctx context.Context, id, retryCount, nextRetryTime int64,
	expectedStatus, status domain.TaskExecutionStatus, progress int32, endTime int64,
	scheduleParams map[string]string, executorNodeID string) error {
	return r.dao.UpdateRetryResult(ctx, id, retryCount, nextRetryTime, expectedStatus.String(),
		status.String(), progress, endTime, scheduleParams, executorNodeID)
}

func (r *taskExecutionRepository) SetRunningState(ctx context.Context, id int64, progress int32, executorNodeID string) error {
	return r.dao.SetRunningState(ctx, id, progress, executorNodeID)
}

func (r *taskExecutionRepository) UpdateRunningProgress(ctx context.Context, id int64, progress int32, executorNodeID string) error {
	return r.dao.UpdateProgress(ctx, id, progress, executorNodeID)
}

func (r *taskExecutionRepository) UpdateScheduleResult(ctx context.Context, id int64,
	expectedStatuses []domain.TaskExecutionStatus, status domain.TaskExecutionStatus,
	progress int32, endTime int64, scheduleParams map[string]string,
	executorNodeID string, taskResult string) (bool, error) {
	return r.dao.UpdateScheduleResult(ctx, id, executionStatusStrings(expectedStatuses), status.String(),
		progress, endTime, scheduleParams, executorNodeID, taskResult)
}

func executionStatusStrings(statuses []domain.TaskExecutionStatus) []string {
	result := make([]string, 0, len(statuses))
	for _, status := range statuses {
		result = append(result, status.String())
	}
	return result
}

func (r *taskExecutionRepository) FindReschedulableExecutions(ctx context.Context, limit int) ([]domain.TaskExecution, error) {
	daoExecutions, err := r.dao.FindReschedulableExecutions(ctx, limit)
	if err != nil {
		return nil, err
	}
	return r.toDomains(daoExecutions)
}

func (r *taskExecutionRepository) FindTimeoutExecutions(ctx context.Context, limit int) ([]domain.TaskExecution, error) {
	daoExecutions, err := r.dao.FindTimeoutExecutions(ctx, limit)
	if err != nil {
		return nil, err
	}
	return r.toDomains(daoExecutions)
}

func (r *taskExecutionRepository) ClaimPullTask(ctx context.Context, serviceName, executorNodeID string,
	handlerNames []string) (domain.TaskExecution, error) {
	daoExec, err := r.dao.ClaimPullTask(ctx, serviceName, executorNodeID, handlerNames)
	if err != nil {
		return domain.TaskExecution{}, err
	}
	return r.toDomain(daoExec)
}

// toEntity 将领域模型转换为DAO模型
func (r *taskExecutionRepository) toEntity(execution domain.TaskExecution) (dao.TaskExecution, error) {
	var requestID sql.NullString
	if execution.RequestID != "" {
		requestID = sql.NullString{String: execution.RequestID, Valid: true}
	}
	var grpcConfig sqlx.JSONColumn[domain.GrpcConfig]
	if execution.Task.GrpcConfig != nil {
		grpcConfig = sqlx.JSONColumn[domain.GrpcConfig]{Val: *execution.Task.GrpcConfig, Valid: true}
	}

	var httpConfig sqlx.JSONColumn[domain.HTTPConfig]
	if execution.Task.HTTPConfig != nil {
		httpConfig = sqlx.JSONColumn[domain.HTTPConfig]{Val: *execution.Task.HTTPConfig, Valid: true}
	}

	var retryConfig sqlx.JSONColumn[domain.RetryConfig]
	if execution.Task.RetryConfig != nil {
		retryConfig = sqlx.JSONColumn[domain.RetryConfig]{Val: *execution.Task.RetryConfig, Valid: true}
	}

	var taskScheduleParams sqlx.JSONColumn[map[string]string]
	if execution.Task.ScheduleParams != nil {
		taskScheduleParams = sqlx.JSONColumn[map[string]string]{Val: execution.Task.ScheduleParams, Valid: true}
	}
	var taskParamOverrides sqlx.JSONColumn[map[string]string]
	if len(execution.ParamOverrides) > 0 {
		taskParamOverrides = sqlx.JSONColumn[map[string]string]{Val: execution.ParamOverrides, Valid: true}
	}

	var artifact sqlx.JSONColumn[[]domain.ArtifactRef]
	if len(execution.Artifacts) > 0 {
		artifact = sqlx.JSONColumn[[]domain.ArtifactRef]{Val: execution.Artifacts, Valid: true}
	}

	var variables sqlx.JSONColumn[domain.ExecutionVariableSet]
	if execution.Variables != nil {
		persisted, err := r.encryptVariables(*execution.Variables)
		if err != nil {
			return dao.TaskExecution{}, err
		}
		variables = sqlx.JSONColumn[domain.ExecutionVariableSet]{Val: persisted, Valid: true}
	}

	var executionRoute sqlx.JSONColumn[domain.ExecutionRoute]
	if execution.Route.Transport != "" {
		executionRoute = sqlx.JSONColumn[domain.ExecutionRoute]{Val: execution.Route, Valid: true}
	}

	var program sqlx.JSONColumn[domain.Program]
	if execution.Program != nil {
		program = sqlx.JSONColumn[domain.Program]{Val: *execution.Program, Valid: true}
	}

	var executorNodeID sql.NullString
	if execution.ExecutorNodeID != "" {
		executorNodeID = sql.NullString{String: execution.ExecutorNodeID, Valid: true}
	}

	return dao.TaskExecution{
		ID:        execution.ID,
		TenantID:  execution.TenantID,
		Source:    execution.Source.String(),
		RequestID: requestID,
		// 从Task展开的冗余字段
		TaskID:                  execution.Task.ID,
		TaskRunnerID:            execution.Task.RunnerID,
		TaskName:                execution.Task.Name,
		TaskType:                execution.Task.Type.String(),
		TaskCronExpr:            execution.Task.CronExpr,
		TaskGrpcConfig:          grpcConfig,
		TaskHTTPConfig:          httpConfig,
		TaskRetryConfig:         retryConfig,
		TaskMaxExecutionSeconds: execution.Task.MaxExecutionSeconds,
		TaskVersion:             execution.Task.Version,
		TaskScheduleNodeID:      execution.Task.ScheduleNodeID,
		TaskScheduleParams:      taskScheduleParams,
		TaskParamOverrides:      taskParamOverrides,
		Artifact:                artifact,
		Variables:               variables,
		Program:                 program,
		ExecutionRoute:          executionRoute,
		// TaskExecution自身字段
		Deadline:        execution.Deadline,
		ExecutorNodeID:  executorNodeID,
		Stime:           execution.StartTime,
		Etime:           execution.EndTime,
		RetryCount:      execution.RetryCount,
		NextRetryTime:   execution.NextRetryTime,
		RunningProgress: execution.RunningProgress,
		Status:          execution.Status.String(),
		ExecMode:        execution.Task.ExecMode.String(),
		Ctime:           execution.CTime,
		Utime:           execution.UTime,
	}, nil
}

// toDomain 将DAO模型转换为领域模型
func (r *taskExecutionRepository) toDomain(daoExecution dao.TaskExecution) (domain.TaskExecution, error) {
	var taskGrpcConfig *domain.GrpcConfig
	if daoExecution.TaskGrpcConfig.Valid {
		taskGrpcConfig = &daoExecution.TaskGrpcConfig.Val
	}

	var taskHTTPConfig *domain.HTTPConfig
	if daoExecution.TaskHTTPConfig.Valid {
		taskHTTPConfig = &daoExecution.TaskHTTPConfig.Val
	}

	var taskRetryConfig *domain.RetryConfig
	if daoExecution.TaskRetryConfig.Valid {
		taskRetryConfig = &daoExecution.TaskRetryConfig.Val
	}

	var taskScheduleParams map[string]string
	if daoExecution.TaskScheduleParams.Valid {
		taskScheduleParams = daoExecution.TaskScheduleParams.Val
	}
	var taskParamOverrides map[string]string
	if daoExecution.TaskParamOverrides.Valid {
		taskParamOverrides = daoExecution.TaskParamOverrides.Val
	}

	var artifacts []domain.ArtifactRef
	if daoExecution.Artifact.Valid {
		artifacts = daoExecution.Artifact.Val
	}

	var variables *domain.ExecutionVariableSet
	if daoExecution.Variables.Valid {
		decrypted, err := r.decryptVariables(daoExecution.Variables.Val)
		if err != nil {
			return domain.TaskExecution{}, err
		}
		variables = &decrypted
	}

	var executionRoute domain.ExecutionRoute
	if daoExecution.ExecutionRoute.Valid {
		executionRoute = daoExecution.ExecutionRoute.Val
	}

	var program *domain.Program
	if daoExecution.Program.Valid {
		program = &daoExecution.Program.Val
	}

	var executorNodeID string
	if daoExecution.ExecutorNodeID.Valid {
		executorNodeID = daoExecution.ExecutorNodeID.String
	}

	return domain.TaskExecution{
		ID:        daoExecution.ID,
		TenantID:  daoExecution.TenantID,
		Source:    domain.TaskExecutionSource(daoExecution.Source),
		RequestID: daoExecution.RequestID.String,
		Task: domain.Task{
			ID:                  daoExecution.TaskID,
			RunnerID:            daoExecution.TaskRunnerID,
			TenantID:            daoExecution.TenantID,
			Name:                daoExecution.TaskName,
			Type:                domain.TaskType(daoExecution.TaskType),
			CronExpr:            daoExecution.TaskCronExpr,
			GrpcConfig:          taskGrpcConfig,
			HTTPConfig:          taskHTTPConfig,
			RetryConfig:         taskRetryConfig,
			MaxExecutionSeconds: daoExecution.TaskMaxExecutionSeconds,
			ScheduleParams:      taskScheduleParams,
			ScheduleNodeID:      daoExecution.TaskScheduleNodeID,
			Version:             daoExecution.TaskVersion,
			ExecMode:            domain.ExecMode(daoExecution.ExecMode),
		},

		Deadline:        daoExecution.Deadline,
		ExecutorNodeID:  executorNodeID,
		StartTime:       daoExecution.Stime,
		EndTime:         daoExecution.Etime,
		RetryCount:      daoExecution.RetryCount,
		NextRetryTime:   daoExecution.NextRetryTime,
		RunningProgress: daoExecution.RunningProgress,
		Status:          domain.TaskExecutionStatus(daoExecution.Status),
		TaskResult:      daoExecution.TaskResult,
		Artifacts:       artifacts,
		Variables:       variables,
		Program:         program,
		Route:           executionRoute,
		ParamOverrides:  taskParamOverrides,
		CTime:           daoExecution.Ctime,
		UTime:           daoExecution.Utime,
	}, nil
}

func (r *taskExecutionRepository) toDomains(source []dao.TaskExecution) ([]domain.TaskExecution, error) {
	result := make([]domain.TaskExecution, 0, len(source))
	for _, item := range source {
		execution, err := r.toDomain(item)
		if err != nil {
			return nil, err
		}
		result = append(result, execution)
	}
	return result, nil
}

func (r *taskExecutionRepository) encryptVariables(source domain.ExecutionVariableSet) (domain.ExecutionVariableSet, error) {
	result := domain.ExecutionVariableSet{Items: append([]domain.RunnerVariable(nil), source.Items...)}
	if r.crypto == nil {
		return result, nil
	}
	for index := range result.Items {
		variable := &result.Items[index]
		if !variable.Secret || variable.Value == "" {
			continue
		}
		value, err := r.crypto.Encrypt(variable.Value)
		if err != nil {
			return domain.ExecutionVariableSet{}, fmt.Errorf("加密执行变量 %q 失败: %w", variable.Key, err)
		}
		variable.Value = value
	}
	return result, nil
}

func (r *taskExecutionRepository) decryptVariables(source domain.ExecutionVariableSet) (domain.ExecutionVariableSet, error) {
	result := domain.ExecutionVariableSet{Items: append([]domain.RunnerVariable(nil), source.Items...)}
	if r.crypto == nil {
		return result, nil
	}
	for index := range result.Items {
		variable := &result.Items[index]
		if !variable.Secret || variable.Value == "" ||
			!strings.HasPrefix(variable.Value, cryptox.EncryptedPrefix) {
			continue
		}
		value, err := r.crypto.Decrypt(variable.Value)
		if err != nil {
			return domain.ExecutionVariableSet{}, fmt.Errorf("解密执行变量 %q 失败: %w", variable.Key, err)
		}
		variable.Value = value
	}
	return result, nil
}
