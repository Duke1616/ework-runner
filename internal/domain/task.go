package domain

import (
	"time"

	taskv1 "github.com/Duke1616/etask/api/proto/gen/etask/task/v1"
	"github.com/Duke1616/etask/pkg/retry"
	"github.com/robfig/cron/v3"
)

// TaskStatus 任务状态
type TaskStatus string

const (
	TaskStatusActive    TaskStatus = "ACTIVE"    // 可调度
	TaskStatusPreempted TaskStatus = "PREEMPTED" // 已抢占
	TaskStatusInactive  TaskStatus = "INACTIVE"  // 停止执行
	TaskStatusCompleted TaskStatus = "COMPLETED" // 已完成（针对一次性任务）
)

func (t TaskStatus) String() string {
	return string(t)
}

// TaskType 任务类型
type TaskType string

const (
	TaskTypeRecurring TaskType = "RECURRING" // 定时任务（循环执行）
	TaskTypeOneTime   TaskType = "ONE_TIME"  // 一次性任务（执行一次后停止）
)

func (tt TaskType) String() string {
	return string(tt)
}

// IsOneTime 判断是否为一次性任务
func (tt TaskType) IsOneTime() bool {
	return tt == TaskTypeOneTime
}

// IsRecurring 判断是否为定时任务
func (tt TaskType) IsRecurring() bool {
	return tt == TaskTypeRecurring
}

type Task struct {
	ID                  int64
	TenantID            int64  // 租户ID，多租户隔离
	BizID               int64  // 业务模块 ID，0 表示独立任务，>0 表示由业务系统创建
	BizKey              string // 业务方唯一标识，如工单号 "order_1448"
	Name                string
	Type                TaskType // 任务类型: RECURRING-定时任务, ONE_TIME-一次性任务
	CronExpr            string   // cron 表达式（定时任务必填，一次性任务可选用于定时触发）
	GrpcConfig          *GrpcConfig
	Program             *ProgramSpec
	HTTPConfig          *HTTPConfig
	RetryConfig         *RetryConfig
	MaxExecutionSeconds int64             // 最大执行秒数，默认24小时
	ScheduleNodeID      string            // 调度节点ID
	ScheduleParams      map[string]string // 调度参数（如分页偏移量、处理进度等）
	NextTime            int64             // 下次执行时间戳
	Status              TaskStatus        // 任务状态
	Version             int64             // 版本号，用于乐观锁
	CTime               int64             // 创建时间戳
	UTime               int64             // 更新时间戳
	ExecMode            ExecMode          // 执行模式：PUSH 或 PULL
	Metadata            map[string]string // 任务参数元数据 (存储绑定关系或模式)
}

// ExecMode 执行模式
type ExecMode string

const (
	ExecModePush ExecMode = "PUSH" // 被动推送模式 (默认)
	ExecModePull ExecMode = "PULL" // 主动拉取模式
)

func (m ExecMode) String() string {
	return string(m)
}

func (m ExecMode) IsPull() bool {
	return m == ExecModePull
}

func (m ExecMode) IsPush() bool {
	return m == ExecModePush
}

func ExecModeFromProto(pb taskv1.ExecMode) ExecMode {
	switch pb {
	case taskv1.ExecMode_PUSH:
		return ExecModePush
	case taskv1.ExecMode_PULL:
		return ExecModePull
	default:
		return ExecModePush // 默认 PUSH
	}
}

func (m ExecMode) ToProto() taskv1.ExecMode {
	switch m {
	case ExecModePush:
		return taskv1.ExecMode_PUSH
	case ExecModePull:
		return taskv1.ExecMode_PULL
	default:
		return taskv1.ExecMode_EXEC_MODE_UNSPECIFIED
	}
}

// RetryConfig 重试配置
type RetryConfig struct {
	MaxRetries      int32
	InitialInterval int64 // 毫秒
	MaxInterval     int64 // 毫秒
}

func (r *RetryConfig) ToRetryComponentConfig() retry.Config {
	return retry.Config{
		Type: "exponential",
		ExponentialBackoff: &retry.ExponentialBackoffConfig{
			InitialInterval: time.Duration(r.InitialInterval) * time.Millisecond,
			MaxInterval:     time.Duration(r.MaxInterval) * time.Millisecond,
			MaxRetries:      r.MaxRetries,
		},
	}
}

// GrpcConfig gRPC配置
type GrpcConfig struct {
	ServiceName string            `json:"serviceName"` // 服务名称
	HandlerName string            `json:"handlerName"` // 执行节点支持的方法名称， 如 shell、python、demo
	Params      map[string]string `json:"params"`      // 传递参数
}

// HTTPConfig HTTP配置
type HTTPConfig struct {
	Endpoint string            `json:"endpoint"`
	Headers  map[string]string `json:"headers"`
	Params   map[string]string `json:"params"`
}

// CalculateNextTime 计算下次执行时间
// - RECURRING 任务: 使用 cron 表达式计算下次执行时间
// - ONE_TIME 任务: 首次使用 cron 计算定时触发时间，执行后返回零值表示不再执行
func (t *Task) CalculateNextTime() (time.Time, error) {
	// 一次性任务：执行完成后不再计算下次时间
	// NOTE: Service 层会在执行完成时将状态设置为 COMPLETED
	if t.Type.IsOneTime() && t.Status == TaskStatusCompleted {
		return time.Time{}, nil
	}

	// 如果没有 cron 表达式，返回零值
	if t.CronExpr == "" {
		return time.Time{}, nil
	}

	// 使用 cron 表达式计算下次执行时间
	p := cron.NewParser(
		cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
	)
	s, err := p.Parse(t.CronExpr)
	if err != nil {
		return time.Time{}, err
	}

	return s.Next(time.Now()), nil
}

// UpdateScheduleParams 在领域模型上定义了“如何更新调度参数”的业务规则
func (t *Task) UpdateScheduleParams(params map[string]string) {
	// 如果传入的 params 是 nil，代表调用者无意进行任何修改。
	if params == nil {
		return // 无操作
	}

	// 如果传入的 params 是一个空的 map，代表业务意图是“重置/清空”。
	if len(params) == 0 {
		t.ScheduleParams = make(map[string]string) // 重置为空
		return
	}
	// 否则，执行“智能合并”逻辑。 如果原始参数是 nil，先初始化
	if t.ScheduleParams == nil {
		t.ScheduleParams = make(map[string]string)
	}
	for k, v := range params {
		t.ScheduleParams[k] = v
	}
}
