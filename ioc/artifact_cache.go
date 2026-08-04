package ioc

import (
	executorartifact "github.com/Duke1616/etask/internal/execution/artifact"
	config "github.com/Duke1616/etask/pkg/config"
	"github.com/Duke1616/etask/sdk/executor/artifact"
)

// InitArtifactPreparer 创建 Agent 和 Executor 共享的制品缓存运行时。
func InitArtifactPreparer() artifact.Preparer {
	var cfg executorartifact.Config
	if err := config.UnmarshalKey("artifact_cache", &cfg); err != nil {
		panic(err)
	}
	return executorartifact.NewRuntime(cfg)
}
