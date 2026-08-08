package task

// Context 的基础结构与身份能力集中在本文件。

import (
	"context"
	"encoding/json"
	"maps"
	"sync"
)

// TaskInfo 描述一次任务执行的只读身份信息。
type TaskInfo struct {
	ExecutionID    int64
	TaskID         int64
	Name           string
	Handler        string
	ExecutorNodeID string
}

// ProgressReporter 将任务进度交给具体运行环境持久化和传播。
type ProgressReporter interface {
	ReportProgress(ctx context.Context, task TaskInfo, progress int32) error
}

// SystemLogger 提供 Executor 和 Handler 的结构化内部诊断日志能力。
// 日志进入宿主进程日志，不进入用户可见的任务执行日志流。
// fields 支持 key/value 对，运行时也可适配自身的字段类型。
type SystemLogger interface {
	Debug(message string, fields ...any)
	Info(message string, fields ...any)
	Warn(message string, fields ...any)
	Error(message string, fields ...any)
}

// ContextOptions 描述创建任务上下文所需的依赖和输入。
type ContextOptions struct {
	Context         context.Context
	Task            TaskInfo
	Params          map[string]string
	Metadata        map[string]string
	Parameters      []Parameter
	Progress        ProgressReporter
	SystemLogger    SystemLogger
	ExecutionLogger ExecutionLogger
}

// Context 向任务处理器提供参数、日志、结果和制品运行目录。
type Context struct {
	ctx           context.Context
	task          TaskInfo
	params        map[string]string
	metadata      map[string]string
	parameters    map[string]Parameter
	artifactRoots ArtifactRoots
	program       *Program

	results map[string]any
	resLock sync.RWMutex

	systemLogger    SystemLogger
	executionLogger ExecutionLogger
	progress        ProgressReporter
}

// Program 返回 Executor 已准备好的本次执行程序输入。
func (c *Context) Program() *Program {
	return c.program
}

// SetProgram 设置由执行引擎准备好的程序输入。
func (c *Context) SetProgram(program *Program) {
	c.program = program
}

// NewContext 创建拥有独立参数快照的任务上下文。
func NewContext(options ContextOptions) *Context {
	// 参数和元数据都复制为任务私有快照，避免 Handler 修改调用方共享数据。
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}
	systemLogger := options.SystemLogger
	if systemLogger == nil {
		systemLogger = noopSystemLogger{}
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
	executionLogger := options.ExecutionLogger
	if executionLogger == nil {
		executionLogger = noopExecutionLogger{}
	}
	executionLogger = newMaskingExecutionLogger(executionLogger, secretMasks(params))
	return &Context{
		ctx: ctx, task: options.Task, params: params, metadata: metadata, parameters: parameters,
		results: make(map[string]any), systemLogger: systemLogger, executionLogger: executionLogger,
		progress: options.Progress,
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

// ArtifactRoots 描述 Executor 为任务准备的不可变制品目录。
type ArtifactRoots struct {
	Default string
	Named   map[string]string
}

// ArtifactRoots 返回由 Executor 准备好的默认层和具名依赖层目录。
func (c *Context) ArtifactRoots() ArtifactRoots {
	return cloneArtifactRoots(c.artifactRoots)
}

// SetArtifactRoots 设置由 Executor 准备好的制品运行目录。
// 该方法只应由执行运行时在调用 Handler 前使用。
func (c *Context) SetArtifactRoots(roots ArtifactRoots) {
	c.artifactRoots = cloneArtifactRoots(roots)
}

func cloneArtifactRoots(roots ArtifactRoots) ArtifactRoots {
	if len(roots.Named) == 0 {
		return roots
	}
	cloned := make(map[string]string, len(roots.Named))
	for name, root := range roots.Named {
		cloned[name] = root
	}
	roots.Named = cloned
	return roots
}

// Log 记录一条用户可见的任务执行日志。
func (c *Context) Log(format string, args ...any) {
	c.executionLogger.Log(format, args...)
}

// AddSecretMasks 注册执行期间产生的敏感值，后续用户日志会自动替换这些值。
func (c *Context) AddSecretMasks(values ...string) {
	if logger, ok := c.executionLogger.(interface{ AddMasks(...string) }); ok {
		logger.AddMasks(values...)
	}
}

// ReportProgress 记录规范化到 0 到 100 的任务进度。
func (c *Context) ReportProgress(progress int) error {
	progress = max(0, min(progress, 100))
	if c.progress == nil {
		c.SystemLogger().Debug("任务未配置进度上报器", "progress", progress)
		return nil
	}
	return c.progress.ReportProgress(c.ctx, c.task, int32(progress))
}

// SystemLogger 返回包含任务身份字段的内部诊断日志组件。
func (c *Context) SystemLogger() SystemLogger { return c.systemLogger }

// Close 刷新任务日志并释放上下文资源。
func (c *Context) Close() {
	if c.executionLogger != nil {
		c.executionLogger.Close()
	}
}

func secretMasks(params map[string]string) []string {
	masks := make([]string, 0)
	for _, parameter := range []string{"variables", "vars"} {
		var variables []Variable
		if json.Unmarshal([]byte(params[parameter]), &variables) != nil {
			continue
		}
		for _, variable := range variables {
			if variable.Secret && variable.Value != "" {
				masks = append(masks, variable.Value)
			}
		}
	}
	return masks
}
