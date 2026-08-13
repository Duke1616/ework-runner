package resource

type ListReq struct {
	Offset  int64  `form:"offset"`
	Limit   int64  `form:"limit"`
	Keyword string `form:"keyword"`
	Kind    string `form:"kind"`
}

type ListResp struct {
	Resources []ResourceVO `json:"resources"`
	Total     int64        `json:"total"`
}

type ResourceVO struct {
	Name           string          `json:"name"`
	Desc           string          `json:"desc"`
	Kind           string          `json:"kind"`
	Transport      string          `json:"transport"`
	DispatchMode   string          `json:"dispatch_mode"`
	IsolationLevel string          `json:"isolation_level"`
	Topic          string          `json:"topic"`
	Handlers       []HandlerDetail `json:"handlers"`
	Nodes          []NodeDetail    `json:"nodes"`
}

type NodeDetail struct {
	ID      string `json:"id"`
	Address string `json:"address"`
}

type HandlerDetail struct {
	Name         string        `json:"name"`
	Desc         string        `json:"desc"`
	Metadata     []ParameterVO `json:"metadata"`
	ProgramKinds []string      `json:"program_kinds,omitempty"`
}

type ParameterVO struct {
	Key                string               `json:"key"`
	Role               string               `json:"role,omitempty"`
	Desc               string               `json:"desc"`
	Secret             bool                 `json:"secret"`
	Required           bool                 `json:"required"`
	RuntimeOverridable bool                 `json:"runtime_overridable"`
	Bindings           map[string]BindingVO `json:"bindings"`
	Default            string               `json:"default"`
}

type BindingVO struct {
	Label       string            `json:"label"`
	Placeholder string            `json:"placeholder"`
	Component   string            `json:"component"`
	Config      map[string]string `json:"config"`
}
