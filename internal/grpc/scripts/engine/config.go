package engine

// Config 描述执行编排层的资源限制。
type Config struct {
	MaxCodeSize      int64
	MaxArgsSize      int64
	MaxVariablesSize int64
	MaxLogLineSize   int
	MaxResultSize    int64
}

// Validate 校验并补全执行编排配置。
func (c *Config) Validate() error {
	if c.MaxCodeSize <= 0 {
		c.MaxCodeSize = 4 << 20
	}
	if c.MaxArgsSize <= 0 {
		c.MaxArgsSize = 1 << 20
	}
	if c.MaxVariablesSize <= 0 {
		c.MaxVariablesSize = 1 << 20
	}
	if c.MaxLogLineSize <= 0 {
		c.MaxLogLineSize = 1 << 20
	}
	if c.MaxResultSize <= 0 {
		c.MaxResultSize = 4 << 20
	}
	return nil
}
