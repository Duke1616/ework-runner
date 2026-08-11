package domain

import (
	"encoding/json"
	"fmt"
	"strings"

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
	Key    string `json:"key"`
	Value  string `json:"value"`
	Secret bool   `json:"secret"`
}

// RunnerExecutionSpec 是执行阶段使用的 Runner 投影。
// Runner.Variables 始终表示 Runner 私有变量，Variables 则是全局变量与私有变量合并后的有效值。
type RunnerExecutionSpec struct {
	Runner    Runner
	Variables []RunnerVariable
}

// Runner 描述脚本模板由哪个执行资源池、哪个 handler、哪些标签承载执行。
type Runner struct {
	ID             int64
	TenantID       int64
	Name           string
	CodebookID     int64
	ProgramKind    ProgramKind
	CodebookSecret string
	Kind           RunnerKind
	Target         string // 执行资源池名称；历史 KAFKA 数据可能暂存 Topic，由仓储兼容解析。
	Handler        string
	Tags           []string
	Action         RunnerAction
	Desc           string
	// ParameterDefaults 保存 Handler 参数的可复用默认值；本次调用参数可以覆盖它们。
	ParameterDefaults map[string]json.RawMessage
	// Variables 仅保存 Runner 私有变量；执行时的有效变量由 RunnerExecutionSpec 提供。
	Variables []RunnerVariable
	CTime     int64
	UTime     int64
}

// Validate 校验执行单元持久化前的必要字段。
func (r *Runner) Validate() error {
	if r.Name == "" {
		return fmt.Errorf("%w: name is empty", errs.ErrInvalidParameter)
	}
	if r.CodebookID <= 0 {
		return fmt.Errorf("%w: codebook_id = %d", errs.ErrInvalidParameter, r.CodebookID)
	}
	if !r.ProgramKind.Valid() {
		return fmt.Errorf("%w: unsupported program kind %s", errs.ErrInvalidParameter, r.ProgramKind)
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
	for key, value := range r.ParameterDefaults {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("%w: parameter default key is empty", errs.ErrInvalidParameter)
		}
		if !json.Valid(value) {
			return fmt.Errorf("%w: parameter default %s is not valid JSON", errs.ErrInvalidParameter, key)
		}
	}
	return nil
}

// IsKindKafka 判断执行单元是否通过 Kafka 派发。
func (r *Runner) IsKindKafka() bool {
	return r.Kind == RunnerKindKafka
}
