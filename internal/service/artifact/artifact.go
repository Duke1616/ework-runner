package artifact

import (
	"context"
	"io"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository"
	"github.com/Duke1616/etask/internal/service/artifact/packer"
	"github.com/Duke1616/etask/pkg/blobstore"
)

type Config struct {
	TempDir string           `mapstructure:"temp_dir" yaml:"temp_dir"`
	Storage blobstore.Config `mapstructure:"storage" yaml:"storage"`
}

// Service 定义制品仓库的发布、查询和执行解析能力。
type Service interface {
	// Publish 将目标代码文件树发布为不可变制品并激活。
	Publish(ctx context.Context, target domain.ArtifactTarget, message string) (domain.ArtifactRelease, error)
	// Active 查询目标当前激活的制品；未发布时返回 nil。
	Active(ctx context.Context, target domain.ArtifactTarget) (*domain.ArtifactRelease, error)
	// Status 查询目标的制品配置、修订号和发布状态。
	Status(ctx context.Context, target domain.ArtifactTarget) (domain.ArtifactStatus, error)
	// List 分页查询目标下的制品发布记录。
	List(ctx context.Context, target domain.ArtifactTarget, offset, limit int64) ([]domain.ArtifactRelease, int64, error)
	// Activate 将目标下指定制品发布记录切换为当前激活版本。
	Activate(ctx context.Context, target domain.ArtifactTarget, id int64) error
	// ResolveExecution 生成固定的 SYSTEM 和当前租户制品库引用，并排除来源项目自身。
	ResolveExecution(ctx context.Context, sourceProjectID int64) ([]domain.ArtifactRef, error)
	// ActiveContents 查询当前执行会使用的激活制品清单，并排除来源项目自身。
	ActiveContents(ctx context.Context, sourceProjectID int64) ([]domain.ArtifactContent, error)
	// ReadFile 从指定制品发布中读取一个文件的不可变内容。
	ReadFile(ctx context.Context, releaseID int64, digest, filePath string) (string, error)
	// Open 根据发布 ID 和内容摘要打开制品数据流。
	Open(ctx context.Context, releaseID int64, digest string) (io.ReadCloser, error)
}

type service struct {
	repo   repository.ArtifactRepository
	store  blobstore.Store
	packer packer.Packer
}

func NewService(repo repository.ArtifactRepository, store blobstore.Store, artifactPacker packer.Packer) Service {
	return &service{repo: repo, store: store, packer: artifactPacker}
}
