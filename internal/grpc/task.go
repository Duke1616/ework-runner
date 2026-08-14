package grpc

import (
	"context"
	"errors"
	"strings"

	notificationv1 "github.com/Duke1616/etask/api/proto/gen/ealert/notification/v1"
	executorv1 "github.com/Duke1616/etask/api/proto/gen/etask/executor/v1"
	taskv1 "github.com/Duke1616/etask/api/proto/gen/etask/task/v1"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	"github.com/Duke1616/etask/internal/service/task"
	"github.com/Duke1616/etask/pkg/grpc/interceptors/bizid"
	"github.com/gotomicro/ego/core/elog"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// TaskServer TaskService gRPC服务实现
type TaskServer struct {
	taskv1.UnimplementedTaskServiceServer
	taskSvc task.Service
	logger  *elog.Component
}

// NewTaskServer 创建 TaskServer 实例
func NewTaskServer(taskSvc task.Service) *TaskServer {
	return &TaskServer{
		taskSvc: taskSvc,
		logger:  elog.DefaultLogger.With(elog.FieldComponentName("grpc.TaskServer")),
	}
}

// CreateTask 创建任务
func (s *TaskServer) CreateTask(ctx context.Context, req *taskv1.CreateTaskRequest) (*taskv1.CreateTaskResponse, error) {
	s.logger.Info("收到创建任务请求",
		elog.String("name", req.GetName()),
		elog.String("type", req.GetType().String()),
		elog.String("cronExpr", req.GetCronExpr()))

	response := &taskv1.CreateTaskResponse{}

	// 从 context 中解析 biz_id
	bizID, err := bizid.FromContext(ctx)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}

	// 将 protobuf 请求转换为 domain 对象
	domainTask := s.toDomainTask(bizID, req)

	// 调用业务服务创建任务
	createdTask, err := s.taskSvc.Create(ctx, domainTask)
	if err != nil {
		if s.isSystemError(err) {
			return nil, status.Errorf(codes.Internal, "系统错误: %v", err)
		}
		// 业务错误逻辑
		response.Code = s.convertToTaskErrorCode(err)
		response.Message = err.Error()
		return response, nil
	}

	response.Id = createdTask.ID
	response.Code = taskv1.TaskErrorCode_SUCCESS
	return response, nil
}

// toDomainTask 将 protobuf CreateTaskRequest 转换为 domain.Task
func (s *TaskServer) toDomainTask(bizID int64, req *taskv1.CreateTaskRequest) domain.Task {
	task := domain.Task{
		BizID:               bizID,
		Name:                req.GetName(),
		Type:                s.toDomainTaskType(req.GetType()),
		CronExpr:            req.GetCronExpr(),
		MaxExecutionSeconds: req.GetMaxExecutionSeconds(),
		ScheduleParams:      req.GetScheduleParams(),
		ExecMode:            domain.ExecModeFromProto(req.GetExecMode()),
		Metadata:            req.GetMetadata(),
		Program:             s.toDomainProgramSpec(req.GetProgram()),
		RunnerID:            req.GetRunnerId(),
		NotificationRules:   s.toDomainExecutionNotifications(req.GetExecutionNotifications()),
		Status:              domain.TaskStatusActive,
		Version:             1,
	}
	if req.GrpcConfig != nil {
		task.GrpcConfig = &domain.GrpcConfig{
			ServiceName: req.GrpcConfig.GetServiceName(),
			HandlerName: req.GrpcConfig.GetHandlerName(),
			Params:      req.GrpcConfig.GetParams(),
		}
	}
	if req.HttpConfig != nil {
		task.HTTPConfig = &domain.HTTPConfig{
			Endpoint: req.HttpConfig.GetEndpoint(),
			Headers:  req.HttpConfig.GetHeaders(),
			Params:   req.HttpConfig.GetParams(),
		}
	}
	if req.RetryConfig != nil {
		task.RetryConfig = &domain.RetryConfig{
			MaxRetries:      req.RetryConfig.GetMaxRetries(),
			InitialInterval: req.RetryConfig.GetInitialInterval(),
			MaxInterval:     req.RetryConfig.GetMaxInterval(),
		}
	}
	return task
}

// toDomainTaskType 将 protobuf TaskType 转换为 domain.TaskType
func (s *TaskServer) toDomainTaskType(t taskv1.TaskType) domain.TaskType {
	switch t {
	case taskv1.TaskType_RECURRING:
		return domain.TaskTypeRecurring
	case taskv1.TaskType_ONE_TIME:
		return domain.TaskTypeOneTime
	default:
		return domain.TaskTypeRecurring
	}
}
func (s *TaskServer) RetryTaskByID(ctx context.Context, req *taskv1.RetryTaskByIDRequest) (*taskv1.RetryTaskResponse, error) {
	s.logger.Info("收到按ID重试任务请求", elog.Int64("id", req.GetId()))
	retryTask, err := s.taskSvc.RetryByID(ctx, req.GetId())
	return s.retryTaskResponse(retryTask, err)
}

func (s *TaskServer) RetryTaskByName(ctx context.Context, req *taskv1.RetryTaskByNameRequest) (*taskv1.RetryTaskResponse, error) {
	s.logger.Info("收到按名称重试任务请求", elog.String("name", req.GetName()))
	retryTask, err := s.taskSvc.RetryByName(ctx, req.GetName())
	return s.retryTaskResponse(retryTask, err)
}

func (s *TaskServer) retryTaskResponse(retryTask domain.Task, err error) (*taskv1.RetryTaskResponse, error) {
	if err != nil {
		if s.isSystemError(err) {
			return nil, status.Errorf(codes.Internal, "重试失败: %v", err)
		}
		return &taskv1.RetryTaskResponse{
			Code: s.convertToTaskErrorCode(err), Message: err.Error(),
		}, nil
	}
	return &taskv1.RetryTaskResponse{Id: retryTask.ID, Code: taskv1.TaskErrorCode_SUCCESS}, nil
}

// GetTask 获取任务
func (s *TaskServer) GetTask(ctx context.Context, req *taskv1.GetTaskRequest) (*taskv1.GetTaskResponse, error) {
	t, err := s.taskSvc.GetByID(ctx, req.GetId())
	if err != nil {
		return nil, status.Error(codes.NotFound, "未找到该任务")
	}

	return &taskv1.GetTaskResponse{
		Task: s.toProtoTask(t),
	}, nil
}

// isSystemError 判断错误是否为系统错误
func (s *TaskServer) isSystemError(err error) bool {
	return errors.Is(err, errs.ErrTaskUpdateNextTimeFailed) ||
		errors.Is(err, errs.ErrTaskUpdateStatusFailed) ||
		errors.Is(err, errs.ErrTaskPreemptFailed)
}

// convertToTaskErrorCode 将错误映射为 gRPC 业务错误码
func (s *TaskServer) convertToTaskErrorCode(err error) taskv1.TaskErrorCode {
	switch {
	case errors.Is(err, errs.ErrTaskNameDuplicate):
		return taskv1.TaskErrorCode_DUPLICATE_NAME
	case errors.Is(err, gorm.ErrRecordNotFound):
		return taskv1.TaskErrorCode_TASK_NOT_FOUND
	case strings.Contains(err.Error(), "运行中"):
		return taskv1.TaskErrorCode_TASK_RUNNING
	default:
		return taskv1.TaskErrorCode_SYSTEM_ERROR
	}
}

// toProtoTask 将 domain.Task 转换为 protobuf Task
func (s *TaskServer) toProtoTask(t domain.Task) *taskv1.Task {
	return &taskv1.Task{
		Id:                     t.ID,
		Name:                   t.Name,
		Type:                   s.toProtoTaskType(t.Type),
		CronExpr:               t.CronExpr,
		MaxExecutionSeconds:    t.MaxExecutionSeconds,
		ScheduleNodeId:         t.ScheduleNodeID,
		ScheduleParams:         t.ScheduleParams,
		NextTime:               t.NextTime,
		Status:                 s.toProtoTaskStatus(t.Status),
		Version:                t.Version,
		Ctime:                  t.CTime,
		Utime:                  t.UTime,
		ExecMode:               t.ExecMode.ToProto(),
		Metadata:               t.Metadata,
		Program:                s.toProtoProgramSpec(t.Program),
		RunnerId:               t.RunnerID,
		GrpcConfig:             s.toProtoGrpcConfig(t.GrpcConfig),
		HttpConfig:             s.toProtoHTTPConfig(t.HTTPConfig),
		RetryConfig:            s.toProtoRetryConfig(t.RetryConfig),
		ExecutionNotifications: s.toProtoExecutionNotifications(t.NotificationRules),
	}
}

func (s *TaskServer) toDomainExecutionNotifications(
	rules []*taskv1.ExecutionNotificationRule) []domain.ExecutionNotificationRule {
	return lo.Map(rules, toDomainExecutionNotification)
}

func (s *TaskServer) toProtoExecutionNotifications(
	rules []domain.ExecutionNotificationRule) []*taskv1.ExecutionNotificationRule {
	return lo.Map(rules, toProtoExecutionNotification)
}

func toDomainExecutionNotification(rule *taskv1.ExecutionNotificationRule, _ int) domain.ExecutionNotificationRule {
	if rule == nil {
		return domain.ExecutionNotificationRule{}
	}
	return domain.ExecutionNotificationRule{
		TriggerStatus: domain.TaskExecutionStatusFromProto(rule.GetTriggerStatus()),
		TemplateSetID: rule.GetTemplateSetId(),
		Enabled:       rule.GetEnabled(),
		Recipients:    lo.Map(rule.GetRecipients(), toDomainNotificationRecipient),
		Channels:      lo.Map(rule.GetChannels(), toDomainNotificationChannel),
	}
}

func toDomainNotificationRecipient(recipient *notificationv1.RecipientSelector, _ int) domain.NotificationRecipient {
	if recipient == nil {
		return domain.NotificationRecipient{}
	}
	return domain.NotificationRecipient{
		Type:      domain.NotificationRecipientType(recipient.GetType().String()),
		TargetIDs: recipient.GetTargetIds(),
	}
}

func toDomainNotificationChannel(channel notificationv1.Channel, _ int) domain.NotificationChannel {
	return domain.NotificationChannel(channel.String())
}

func toProtoExecutionNotification(rule domain.ExecutionNotificationRule, _ int) *taskv1.ExecutionNotificationRule {
	return &taskv1.ExecutionNotificationRule{
		TriggerStatus: toProtoExecutionStatus(rule.TriggerStatus),
		TemplateSetId: rule.TemplateSetID,
		Enabled:       rule.Enabled,
		Recipients:    lo.Map(rule.Recipients, toProtoNotificationRecipient),
		Channels:      lo.Map(rule.Channels, toProtoNotificationChannel),
	}
}

func toProtoNotificationRecipient(recipient domain.NotificationRecipient, _ int) *notificationv1.RecipientSelector {
	return &notificationv1.RecipientSelector{
		Type: notificationv1.RecipientSelectorType(
			notificationv1.RecipientSelectorType_value[string(recipient.Type)]),
		TargetIds: recipient.TargetIDs,
	}
}

func toProtoNotificationChannel(channel domain.NotificationChannel, _ int) notificationv1.Channel {
	return notificationv1.Channel(notificationv1.Channel_value[string(channel)])
}

func toProtoExecutionStatus(status domain.TaskExecutionStatus) executorv1.ExecutionStatus {
	switch status {
	case domain.TaskExecutionStatusSuccess:
		return executorv1.ExecutionStatus_SUCCESS
	case domain.TaskExecutionStatusFailed:
		return executorv1.ExecutionStatus_FAILED
	case domain.TaskExecutionStatusCancelled:
		return executorv1.ExecutionStatus_CANCELLED
	default:
		return executorv1.ExecutionStatus_UNKNOWN
	}
}

func (s *TaskServer) toProtoGrpcConfig(cfg *domain.GrpcConfig) *taskv1.GrpcConfig {
	if cfg == nil {
		return nil
	}
	return &taskv1.GrpcConfig{
		ServiceName: cfg.ServiceName,
		HandlerName: cfg.HandlerName,
		Params:      cfg.Params,
	}
}

func (s *TaskServer) toDomainProgramSpec(spec *taskv1.ProgramSpec) *domain.ProgramSpec {
	if spec == nil {
		return nil
	}
	switch source := spec.Source.(type) {
	case *taskv1.ProgramSpec_Inline:
		inline := &domain.InlineProgramSpec{}
		if source.Inline != nil {
			switch value := source.Inline.Source.(type) {
			case *taskv1.InlineProgramSpec_Code:
				inline.Code = value.Code
			case *taskv1.InlineProgramSpec_CodebookId:
				inline.CodebookID = value.CodebookId
			}
		}
		return &domain.ProgramSpec{Kind: domain.ProgramInline, Inline: inline}
	case *taskv1.ProgramSpec_Project:
		project := &domain.ProjectProgramSpec{}
		if source.Project != nil {
			project.EntryCodebookID = source.Project.EntryCodebookId
		}
		return &domain.ProgramSpec{Kind: domain.ProgramProject, Project: project}
	default:
		return &domain.ProgramSpec{}
	}
}

func (s *TaskServer) toProtoProgramSpec(spec *domain.ProgramSpec) *taskv1.ProgramSpec {
	if spec == nil {
		return nil
	}
	result := &taskv1.ProgramSpec{}
	switch spec.Kind {
	case domain.ProgramInline:
		inline := &taskv1.InlineProgramSpec{}
		if spec.Inline != nil {
			if spec.Inline.CodebookID > 0 {
				inline.Source = &taskv1.InlineProgramSpec_CodebookId{CodebookId: spec.Inline.CodebookID}
			} else {
				inline.Source = &taskv1.InlineProgramSpec_Code{Code: spec.Inline.Code}
			}
		}
		result.Source = &taskv1.ProgramSpec_Inline{Inline: inline}
	case domain.ProgramProject:
		project := &taskv1.ProjectProgramSpec{}
		if spec.Project != nil {
			project.EntryCodebookId = spec.Project.EntryCodebookID
		}
		result.Source = &taskv1.ProgramSpec_Project{Project: project}
	}
	return result
}

func (s *TaskServer) toProtoHTTPConfig(cfg *domain.HTTPConfig) *taskv1.HTTPConfig {
	if cfg == nil {
		return nil
	}
	return &taskv1.HTTPConfig{
		Endpoint: cfg.Endpoint,
		Headers:  cfg.Headers,
		Params:   cfg.Params,
	}
}

func (s *TaskServer) toProtoRetryConfig(cfg *domain.RetryConfig) *taskv1.RetryConfig {
	if cfg == nil {
		return nil
	}
	return &taskv1.RetryConfig{
		MaxRetries:      cfg.MaxRetries,
		InitialInterval: cfg.InitialInterval,
		MaxInterval:     cfg.MaxInterval,
	}
}

// toProtoTaskType 将 domain.TaskType 转换为 protobuf TaskType
func (s *TaskServer) toProtoTaskType(t domain.TaskType) taskv1.TaskType {
	switch t {
	case domain.TaskTypeRecurring:
		return taskv1.TaskType_RECURRING
	case domain.TaskTypeOneTime:
		return taskv1.TaskType_ONE_TIME
	default:
		return taskv1.TaskType_TASK_TYPE_UNSPECIFIED
	}
}

// toProtoTaskStatus 将 domain.TaskStatus 转换为 protobuf TaskStatus
func (s *TaskServer) toProtoTaskStatus(t domain.TaskStatus) taskv1.TaskStatus {
	switch t {
	case domain.TaskStatusActive:
		return taskv1.TaskStatus_ACTIVE
	case domain.TaskStatusPreempted:
		return taskv1.TaskStatus_PREEMPTED
	case domain.TaskStatusInactive:
		return taskv1.TaskStatus_INACTIVE
	case domain.TaskStatusCompleted:
		return taskv1.TaskStatus_COMPLETED
	default:
		return taskv1.TaskStatus_TASK_STATUS_UNSPECIFIED
	}
}
