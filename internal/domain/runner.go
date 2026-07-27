package domain

import (
	"fmt"

	"github.com/Duke1616/etask/internal/errs"
)

// RunnerKind 描述脚本执行单元的派发通道类型。
type RunnerKind string

const (
	// RunnerKindKafka 表示通过消息队列 Topic 派发执行。
	RunnerKindKafka RunnerKind = "KAFKA"
	// RunnerKindGRPC 表示通过 etask gRPC 执行节点派发执行。
	RunnerKindGRPC RunnerKind = "GRPC"
)

// String 返回派发通道类型的字符串值。
func (k RunnerKind) String() string {
	return string(k)
}

// IsValid 判断执行单元通道类型是否受支持。
func (k RunnerKind) IsValid() bool {
	return k == RunnerKindGRPC || k == RunnerKindKafka
}

// Transport 返回执行单元期望使用的传输通道。
func (k RunnerKind) Transport() ExecutionTransport {
	if k == RunnerKindKafka {
		return ExecutionTransportMQ
	}
	return ExecutionTransportGRPC
}

// RunnerAction 描述执行单元的注册状态。
type RunnerAction uint8

const (
	// RunnerActionRegistered 表示执行单元已注册且可用。
	RunnerActionRegistered RunnerAction = 1
	// RunnerActionUnregistered 表示执行单元已注销且不可用。
	RunnerActionUnregistered RunnerAction = 2
)

// Uint8 返回用于持久化的状态值。
func (a RunnerAction) Uint8() uint8 {
	return uint8(a)
}

// RunnerVariable 表示执行脚本时透传的默认变量。
type RunnerVariable struct {
	Key    string
	Value  string
	Secret bool
}

// Runner 描述脚本模板由哪个执行资源池、哪个 handler、哪些标签承载执行。
type Runner struct {
	ID             int64
	TenantID       int64
	Name           string
	CodebookID     int64
	CodebookSecret string
	Kind           RunnerKind
	Target         string // 执行资源池名称；历史 KAFKA 数据可能暂存 Topic，由仓储兼容解析。
	Handler        string
	Tags           []string
	Action         RunnerAction
	Desc           string
	Variables      []RunnerVariable
	CTime          int64
	UTime          int64
}

// Validate 校验执行单元持久化前的必要字段。
func (r *Runner) Validate() error {
	if r.Name == "" {
		return fmt.Errorf("%w: name is empty", errs.ErrInvalidParameter)
	}
	if r.CodebookID <= 0 {
		return fmt.Errorf("%w: codebook_id = %d", errs.ErrInvalidParameter, r.CodebookID)
	}
	if !r.Kind.IsValid() {
		return fmt.Errorf("%w: unsupported kind %s", errs.ErrInvalidParameter, r.Kind)
	}
	if r.Target == "" {
		return fmt.Errorf("%w: target is empty", errs.ErrInvalidParameter)
	}
	if r.Handler == "" {
		return fmt.Errorf("%w: handler is empty", errs.ErrInvalidParameter)
	}
	return nil
}

// IsKindKafka 判断执行单元是否通过 Kafka 派发。
func (r *Runner) IsKindKafka() bool {
	return r.Kind == RunnerKindKafka
}
