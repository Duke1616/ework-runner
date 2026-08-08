package shell

import "github.com/Duke1616/etask/internal/grpc/scripts/engine"

// Config 配置 Shell Handler。
type Config struct {
	Enabled bool   `mapstructure:"enabled" yaml:"enabled"`
	Binary  string `mapstructure:"binary" yaml:"binary"`
}

// IsEnabled 返回是否注册 Shell Handler。
func (c Config) IsEnabled() bool { return c.Enabled }

// Build 构造 Shell Adapter。
func (c Config) Build() (engine.Adapter, error) {
	return New(c.Binary), nil
}

var _ engine.AdapterFactory = Config{}
