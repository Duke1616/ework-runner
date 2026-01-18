package task

type CreateTaskReq struct {
	Name                string            `json:"name"`
	Type                string            `json:"type"`      // 任务类型: RECURRING-定时任务, ONE_TIME-一次性任务
	CronExpr            string            `json:"cron_expr"` // cron 表达式（定时任务必填，一次性任务可选用于定时触发）
	GrpcConfig          *GrpcConfig       `json:"grpc_config"`
	HTTPConfig          *HTTPConfig       `json:"http_config"`
	RetryConfig         *RetryConfig      `json:"retry_config"`
	MaxExecutionSeconds int64             `json:"max_execution_seconds"` // 最大执行秒数，默认24小时
	ScheduleParams      map[string]string `json:"schedule_params"`       // 调度参数（如分页偏移量、处理进度等）
}

type GrpcConfig struct {
	ServiceName string            `json:"service_name"` // 服务名称
	HandlerName string            `json:"handler_name"` // 执行节点支持的方法名称， 如 shell、python、demo
	Params      map[string]string `json:"params"`       // 传递参数
}

type HTTPConfig struct {
	Endpoint string            `json:"endpoint"`
	Params   map[string]string `json:"params"`
}

type RetryConfig struct {
	MaxRetries      int32 `json:"max_retries"`
	InitialInterval int64 `json:"initial_interval"` // 毫秒
	MaxInterval     int64 `json:"max_interval"`     // 毫秒
}
