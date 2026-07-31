package task

// Context 的基础结构与身份能力集中在本文件。

import (
	"context"
	"encoding/json"
	"maps"
	"sync"

	reporterv1 "github.com/Duke1616/etask/api/proto/gen/etask/reporter/v1"
	"github.com/gotomicro/ego/core/elog"
)

// TaskInfo 描述一次任务执行的只读身份信息。
type TaskInfo struct {
	ExecutionID int64
	TaskID      int64
	Name        string
	Handler     string
}

// ContextOptions 描述创建任务上下文所需的依赖和输入。
type ContextOptions struct {
	Context    context.Context
	Task       TaskInfo
	Params     map[string]string
	Metadata   map[string]string
	Parameters []Parameter
	Reporter   reporterv1.ReporterServiceClient
	Logger     *elog.Component
	TaskLogger TaskLogger
}

// Context 向任务处理器提供参数、日志、结果和制品运行目录。
type Context struct {
	ctx           context.Context
	task          TaskInfo
	params        map[string]string
	metadata      map[string]string
	parameters    map[string]Parameter
	artifactRoots ArtifactRoots

	results map[string]any
	resLock sync.RWMutex

	logger     *elog.Component
	taskLogger TaskLogger
}

// NewContext 创建拥有独立参数快照的任务上下文。
func NewContext(options ContextOptions) *Context {
	// 参数和元数据都复制为任务私有快照，避免 Handler 修改调用方共享数据。
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}
	logger := options.Logger
	if logger == nil {
		logger = elog.DefaultLogger
	}
	params := maps.Clone(options.Params)
	if params == nil {
		params = make(map[string]string)
	}
	metadata := maps.Clone(options.Metadata)
	if metadata == nil {
		metadata = make(map[string]string)
	}
	parameters := make(map[string]Parameter, len(options.Parameters))
	for _, parameter := range options.Parameters {
		parameters[parameter.Key] = parameter
	}
	// 日志缓冲和传输由具体实现负责，敏感变量统一在最外层脱敏。
	taskLogger := options.TaskLogger
	if taskLogger == nil {
		taskLogger = newTaskLogger(ctx, options.Task.ExecutionID, options.Reporter, logger)
	}
	taskLogger = newMaskingTaskLogger(taskLogger, secretMasks(params))
	return &Context{
		ctx: ctx, task: options.Task, params: params, metadata: metadata, parameters: parameters,
		results: make(map[string]any), logger: logger, taskLogger: taskLogger,
	}
}

// Context 返回承载取消信号和租户信息的原生上下文。
func (c *Context) Context() context.Context {
	return c.ctx
}

// ExecutionID 返回本次执行 ID。
func (c *Context) ExecutionID() int64 {
	return c.task.ExecutionID
}

// TaskID 返回任务 ID。
func (c *Context) TaskID() int64 {
	return c.task.TaskID
}

// TaskName 返回任务名称。
func (c *Context) TaskName() string {
	return c.task.Name
}

// HandlerName 返回当前处理器名称。
func (c *Context) HandlerName() string {
	return c.task.Handler
}

// Params 返回任务参数快照。
func (c *Context) Params() map[string]string {
	return maps.Clone(c.params)
}

// TaskLogger 返回当前任务日志实现，供共享执行引擎复用。
func (c *Context) TaskLogger() TaskLogger {
	return c.taskLogger
}

// MergeResultJSON 将共享执行引擎产生的结构化结果合并到当前 Context。
func (c *Context) MergeResultJSON(value string) {
	if value == "" {
		return
	}
	var result map[string]any
	if json.Unmarshal([]byte(value), &result) != nil {
		return
	}
	c.resLock.Lock()
	defer c.resLock.Unlock()
	for key, val := range result {
		c.results[key] = val
	}
}

// ArtifactRoots 描述 Executor 为任务准备的制品运行目录。
type ArtifactRoots struct {
	Default      string
	Dependencies string
}

// ArtifactRoots 返回由 Executor 准备好的默认层和具名依赖层目录。
func (c *Context) ArtifactRoots() ArtifactRoots {
	return c.artifactRoots
}

// SetArtifactRoots 设置由 Executor 准备好的制品运行目录。
// 该方法只应由执行运行时在调用 Handler 前使用。
func (c *Context) SetArtifactRoots(roots ArtifactRoots) {
	c.artifactRoots = roots
}

// Log 记录一条任务日志。
func (c *Context) Log(format string, args ...any) {
	c.taskLogger.Log(format, args...)
}

// ReportProgress 记录规范化到 0 到 100 的任务进度。
func (c *Context) ReportProgress(progress int) error {
	progress = max(0, min(progress, 100))
	c.Logger().Debug("进度上报", elog.Int("progress", progress))
	return nil
}

// Logger 返回包含任务身份字段的系统日志组件。
func (c *Context) Logger() *elog.Component {
	return c.logger.With(
		elog.Int64("executionID", c.task.ExecutionID),
		elog.Int64("taskID", c.task.TaskID),
		elog.String("taskName", c.task.Name),
	)
}

// Close 刷新任务日志并释放上下文资源。
func (c *Context) Close() {
	if c.taskLogger != nil {
		c.taskLogger.Close()
	}
}

func secretMasks(params map[string]string) []string {
	var variables []Variable
	if json.Unmarshal([]byte(params["variables"]), &variables) != nil {
		return nil
	}
	masks := make([]string, 0, len(variables))
	for _, variable := range variables {
		if variable.Secret && variable.Value != "" {
			masks = append(masks, variable.Value)
		}
	}
	return masks
}
