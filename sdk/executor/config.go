package executor

import "fmt"

// Config Executor 配置
type Config struct {
	NodeID              string   // 节点ID,如 "cmdb-executor-001"
	ServiceName         string   // 服务名,如 "cmdb", "ticket"
	Addr                string   // 监听地址,如 "0.0.0.0:9020"
	EtcdEndpoints       []string // etcd 地址列表
	ReporterServiceName string   // Reporter 服务名,用于服务发现,如 "scheduler"
}

// Validate 验证配置
func (c *Config) Validate() error {
	if c.NodeID == "" {
		return fmt.Errorf("NodeID 不能为空")
	}
	if c.ServiceName == "" {
		return fmt.Errorf("ServiceName 不能为空")
	}
	if c.Addr == "" {
		return fmt.Errorf("地址不能为空")
	}
	if len(c.EtcdEndpoints) == 0 {
		return fmt.Errorf("EtcdEndpoints 不能为空")
	}
	if c.ReporterServiceName == "" {
		return fmt.Errorf("ReporterServiceName 不能为空")
	}
	return nil
}
