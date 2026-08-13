// Package task 实现 Executor 的任务处理契约和上下文。
package task

// Variable 描述传给任务处理器的一个变量。
type Variable struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Secret bool   `json:"secret"`
}

// VariableSet 表示一次执行提供的完整变量快照。
type VariableSet struct {
	Items []Variable `json:"items"`
}

// ParameterRole 描述参数在执行协议中的语义用途。
type ParameterRole string

const (
	// ParameterRoleArgs 表示参数承载统一脚本入参。
	ParameterRoleArgs ParameterRole = "args"
	// ParameterRoleVariables 表示参数承载统一变量集合。
	ParameterRoleVariables ParameterRole = "variables"
)

// Parameter 描述任务处理器支持的一个参数。
type Parameter struct {
	Key                string             `json:"key"`
	Role               ParameterRole      `json:"role,omitempty"`
	Desc               string             `json:"desc"`
	Secret             bool               `json:"secret"`
	Required           bool               `json:"required"`
	RuntimeOverridable bool               `json:"runtime_overridable"`
	Bindings           map[string]Binding `json:"bindings"`
	Default            string             `json:"default"`
}

// ProgramHandler 声明 Handler 支持的程序来源类型。
// 未实现该接口的 Handler 不要求调度任务提供 ProgramSpec。
type ProgramHandler interface {
	// ProgramKinds 返回 Handler 支持的程序来源类型。
	ProgramKinds() []ProgramKind
}

// Binding 定义运行阶段的参数绑定解析行为。
type Binding interface {
	// Resolve 根据原始值解析并返回任务处理器实际使用的参数。
	Resolve(ctx *Context, value string) (string, error)
}

// BindingOption 描述前端渲染配置和可选的运行阶段解析函数。
type BindingOption struct {
	Label       string                                           `json:"label"`
	Placeholder string                                           `json:"placeholder"`
	Component   string                                           `json:"component"`
	Config      map[string]string                                `json:"config"`
	Resolver    func(ctx *Context, value string) (string, error) `json:"-"`
}

// Resolve 使用自定义解析函数处理参数；未配置时直接返回原值。
func (b *BindingOption) Resolve(ctx *Context, value string) (string, error) {
	if b.Resolver != nil {
		return b.Resolver(ctx, value)
	}
	return value, nil
}

// TaskHandler 定义 Executor 可以调度的一类任务。
type TaskHandler interface {
	// Name 返回处理器唯一名称。
	Name() string
	// Desc 返回处理器用途说明。
	Desc() string
	// Metadata 返回处理器支持的参数定义。
	Metadata() []Parameter
	// Run 执行一次任务。
	Run(*Context) error
}

// HandlerMeta 是用于注册中心和管理页面展示的处理器元数据。
type HandlerMeta struct {
	Name         string        `json:"name"`
	Desc         string        `json:"desc"`
	Metadata     []Parameter   `json:"metadata"`
	ProgramKinds []ProgramKind `json:"program_kinds,omitempty"`
}
