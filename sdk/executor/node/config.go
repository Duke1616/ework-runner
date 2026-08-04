// Package node 提供标准 gRPC Executor 节点运行时。
package node

import (
	grpcpkg "github.com/Duke1616/etask/pkg/grpc"
	"github.com/Duke1616/etask/sdk/executor/internal/runtime"
)

const (
	RoleName           = runtime.RoleName
	ModePush           = runtime.ModePush
	ModePull           = runtime.ModePull
	IsolationShared    = runtime.IsolationShared
	IsolationDedicated = runtime.IsolationDedicated
)

// Config 描述标准 Executor 节点的运行配置。
type Config struct {
	Mode           string
	Desc           string
	IsolationLevel string
	Server         grpcpkg.ServerConfig
	Client         grpcpkg.ClientConfig
}

func (c Config) runtimeConfig() runtime.Config {
	return runtime.Config{
		Mode: c.Mode, Desc: c.Desc, IsolationLevel: c.IsolationLevel,
		Server: c.Server, Client: c.Client,
	}
}
