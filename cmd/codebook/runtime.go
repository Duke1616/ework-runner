package codebookcmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository"
	"github.com/Duke1616/etask/internal/repository/dao"
	artifactsvc "github.com/Duke1616/etask/internal/service/artifact"
	"github.com/Duke1616/etask/internal/service/artifact/packer"
	codebooksvc "github.com/Duke1616/etask/internal/service/codebook"
	"github.com/Duke1616/etask/ioc"
	"github.com/Duke1616/etask/pkg/blobstore"
	config "github.com/Duke1616/etask/pkg/config"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

const systemCodebookAuthorUserID int64 = 1

type commandRuntime struct {
	db          *gorm.DB
	artifactSvc artifactsvc.Service
}

func newRuntime() (*commandRuntime, error) {
	if strings.TrimSpace(viper.GetString("mysql.dsn")) == "" {
		return nil, fmt.Errorf("mysql.dsn 不能为空")
	}
	artifactConfig, artifactStore, err := loadArtifactStorage()
	if err != nil {
		return nil, err
	}
	db := ioc.InitDB()
	return &commandRuntime{
		db:          db,
		artifactSvc: newArtifactService(db, artifactConfig, artifactStore),
	}, nil
}

func (r *commandRuntime) importSystem(ctx context.Context,
	plan codebooksvc.SystemImportPlan) (codebooksvc.SystemImportResult, domain.ArtifactRelease, error) {
	ctx = systemOperationContext(ctx)
	var result codebooksvc.SystemImportResult
	// 数据库事务只执行导入计划，不在事务内扫描本地目录或上传对象。
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var importErr error
		result, importErr = codebooksvc.NewSystemImporter(newCodebookService(tx)).Import(ctx, plan)
		return importErr
	})
	if err != nil {
		return codebooksvc.SystemImportResult{}, domain.ArtifactRelease{}, err
	}

	// 导入提交后立即发布 SYSTEM 制品，使 Executor 只消费完整的已提交源码树。
	release, err := r.artifactSvc.Publish(ctx, domain.ArtifactTarget{Scope: domain.CodebookScopeSystem},
		fmt.Sprintf("SYSTEM 组件库导入: %s", result.RootName))
	if err != nil {
		return result, domain.ArtifactRelease{}, fmt.Errorf("SYSTEM 组件库已导入，但制品发布失败: %w", err)
	}
	return result, release, nil
}

func systemOperationContext(ctx context.Context) context.Context {
	ctx = ctxutil.WithTenantID(ctx, ctxutil.SystemTenantID)
	return ctxutil.WithUserID(ctx, systemCodebookAuthorUserID)
}

func loadArtifactStorage() (artifactsvc.Config, blobstore.Store, error) {
	var cfg artifactsvc.Config
	if err := config.UnmarshalKey("artifact", &cfg); err != nil {
		return artifactsvc.Config{}, nil, fmt.Errorf("读取制品仓库配置失败: %w", err)
	}
	store, err := blobstore.New(cfg.Storage)
	if err != nil {
		return artifactsvc.Config{}, nil, fmt.Errorf("初始化制品仓库存储失败: %w", err)
	}
	return cfg, store, nil
}

func newCodebookService(db *gorm.DB) codebooksvc.Service {
	codebookDAO := dao.NewGORMCodebookDAO(db)
	projectDAO := dao.NewGORMCodebookProjectDAO(db)
	repo := repository.NewCodebookRepository(codebookDAO, projectDAO)
	return codebooksvc.NewService(repo)
}

func newArtifactService(db *gorm.DB, cfg artifactsvc.Config, store blobstore.Store) artifactsvc.Service {
	codebookDAO := dao.NewGORMCodebookDAO(db)
	projectDAO := dao.NewGORMCodebookProjectDAO(db)
	artifactDAO := dao.NewGORMArtifactDAO(db)
	repo := repository.NewArtifactRepository(artifactDAO, codebookDAO, projectDAO)
	return artifactsvc.NewService(repo, store, packer.New(cfg.TempDir))
}
