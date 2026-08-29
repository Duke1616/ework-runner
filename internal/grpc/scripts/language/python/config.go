package python

import (
	"github.com/Duke1616/etask/internal/grpc/scripts/engine"
)

// Config 配置 Python Handler。
type Config struct {
	Enabled bool   `mapstructure:"enabled" yaml:"enabled"`
	Binary  string `mapstructure:"binary" yaml:"binary"`
}

// IsEnabled 返回是否注册 Python Handler。
func (c Config) IsEnabled() bool { return c.Enabled }

// Build 构造 Python Adapter。
func (c Config) Build() (engine.Adapter, error) {
	return New(c.Binary), nil
}

var _ engine.AdapterFactory = Config{}
