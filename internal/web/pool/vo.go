package pool

type ListPoolsReq struct {
	Offset       int64  `json:"offset"`
	Limit        int64  `json:"limit"`
	Keyword      string `json:"keyword"`
	Kind         string `json:"kind"`
	Transport    string `json:"transport"`
	DispatchMode string `json:"dispatch_mode"`
	Status       string `json:"status"`
}

type BindReq struct {
	PoolName     string   `json:"pool_name"`
	HandlerName  string   `json:"handler_name"`
	HandlerNames []string `json:"handler_names"`
	Desc         string   `json:"desc"`
}

type BindingKeyReq struct {
	PoolName    string `json:"pool_name"`
	HandlerName string `json:"handler_name"`
}

type ListBindingsReq struct {
	PoolName   string `json:"pool_name"`
	Status     string `json:"status"`
	AllTenants bool   `json:"all_tenants"`
}

type BindingVO struct {
	ID          int64  `json:"id"`
	TenantID    int64  `json:"tenant_id"`
	PoolName    string `json:"pool_name"`
	HandlerName string `json:"handler_name"`
	Status      string `json:"status"`
	Desc        string `json:"desc"`
	CTime       int64  `json:"ctime"`
	UTime       int64  `json:"utime"`
}

type PoolVO struct {
	ID             int64             `json:"id"`
	Name           string            `json:"name"`
	Kind           string            `json:"kind"`
	Transport      string            `json:"transport"`
	DispatchMode   string            `json:"dispatch_mode"`
	IsolationLevel string            `json:"isolation_level"`
	Desc           string            `json:"desc"`
	Status         string            `json:"status"`
	Metadata       map[string]string `json:"metadata"`
	CTime          int64             `json:"ctime"`
	UTime          int64             `json:"utime"`
}

type ListPoolsResp struct {
	Total int64    `json:"total"`
	Pools []PoolVO `json:"pools"`
}

type ListBindingsResp struct {
	Bindings []BindingVO `json:"bindings"`
}
