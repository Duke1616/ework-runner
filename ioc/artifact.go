package ioc

import (
	artifactSvc "github.com/Duke1616/etask/internal/service/artifact"
	artifactPacker "github.com/Duke1616/etask/internal/service/artifact/packer"
	"github.com/Duke1616/etask/pkg/blobstore"
	config "github.com/Duke1616/etask/pkg/config"
)

func InitArtifactConfig() artifactSvc.Config {
	var cfg artifactSvc.Config
	if err := config.UnmarshalKey("artifact", &cfg); err != nil {
		panic(err)
	}
	return cfg
}

func InitArtifactStore(cfg artifactSvc.Config) blobstore.Store {
	store, err := blobstore.New(cfg.Storage)
	if err != nil {
		panic(err)
	}
	return store
}

// InitArtifactPacker 创建制品发布使用的打包器。
func InitArtifactPacker(cfg artifactSvc.Config) artifactPacker.Packer {
	return artifactPacker.New(cfg.TempDir)
}
