package executor

import (
	"strconv"

	reporterv1 "github.com/Duke1616/ework-runner/api/proto/gen/reporter/v1"
	"github.com/gotomicro/ego/core/elog"
)

// TaskHandler 任务处理函数接口
type TaskHandler interface {
	Name() string
	Run(*Context) error
}

// Context 任务执行上下文
type Context struct {
	ExecutionID int64
	TaskID      int64
	TaskName    string
	Params      map[string]string

	// 内部字段
	reporter reporterv1.ReporterServiceClient
	logger   *elog.Component
}

// newContext 创建上下文(内部使用)
func newContext(eid, taskID int64, taskName string, params map[string]string,
	reporter reporterv1.ReporterServiceClient, logger *elog.Component) *Context {
	return &Context{
		ExecutionID: eid,
		TaskID:      taskID,
		TaskName:    taskName,
		Params:      params,
		reporter:    reporter,
		logger:      logger,
	}
}

// Param 获取字符串参数
func (c *Context) Param(key string) string {
	return c.Params[key]
}

// ParamInt 获取整数参数
func (c *Context) ParamInt(key string) int {
	val := c.Params[key]
	if val == "" {
		return 0
	}
	i, _ := strconv.Atoi(val)
	return i
}

// ParamInt64 获取 int64 参数
func (c *Context) ParamInt64(key string) int64 {
	val := c.Params[key]
	if val == "" {
		return 0
	}
	i, _ := strconv.ParseInt(val, 10, 64)
	return i
}

// ParamBool 获取布尔参数
func (c *Context) ParamBool(key string) bool {
	val := c.Params[key]
	if val == "" {
		return false
	}
	b, _ := strconv.ParseBool(val)
	return b
}

// ReportProgress 上报进度 (可选)
// NOTE: 对于没有进度的任务,不调用此方法也完全OK
func (c *Context) ReportProgress(progress int) error {
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}

	// TODO: 实现进度上报
	// 当前简化版本,可以后续增强
	c.logger.Debug("进度上报", elog.Int("progress", progress))
	return nil
}

// Logger 获取日志组件
func (c *Context) Logger() *elog.Component {
	return c.logger.With(
		elog.Int64("executionID", c.ExecutionID),
		elog.Int64("taskID", c.TaskID),
		elog.String("taskName", c.TaskName),
	)
}
