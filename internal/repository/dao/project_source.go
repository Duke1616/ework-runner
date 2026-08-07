package dao

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ProjectSource 映射系统自动生成的不可变项目源码。
type ProjectSource struct {
	ID             int64  `gorm:"primaryKey;type:bigint;autoIncrement"`
	TenantID       int64  `gorm:"type:bigint unsigned;not null;index;uniqueIndex:uniq_project_source,priority:1"`
	ProjectID      int64  `gorm:"type:bigint;not null;index;uniqueIndex:uniq_project_source,priority:2"`
	SourceRevision int64  `gorm:"type:bigint;not null;uniqueIndex:uniq_project_source,priority:3"`
	Digest         string `gorm:"type:char(64);not null;uniqueIndex:uniq_project_source,priority:4"`
	BlobChecksum   string `gorm:"type:char(64);not null"`
	ObjectKey      string `gorm:"type:varchar(512);not null"`
	Size           int64  `gorm:"column:compressed_size;type:bigint;not null"`
	Format         string `gorm:"type:varchar(32);not null"`
	FormatVersion  int32  `gorm:"type:int;not null"`
	CTime          int64  `gorm:"not null"`
}

// TableName 保留既有表名，代码概念调整不触发数据库迁移。
func (ProjectSource) TableName() string { return "codebook_project_snapshots" }

type ProjectSourceDAO interface {
	FindByRevision(ctx context.Context, projectID, sourceRevision int64) (ProjectSource, error)
	FindByID(ctx context.Context, id int64) (ProjectSource, error)
	Create(ctx context.Context, source ProjectSource) (ProjectSource, error)
}

type GORMProjectSourceDAO struct{ db *gorm.DB }

func NewGORMProjectSourceDAO(db *gorm.DB) ProjectSourceDAO {
	return &GORMProjectSourceDAO{db: db}
}

func (g *GORMProjectSourceDAO) FindByRevision(ctx context.Context,
	projectID, sourceRevision int64) (ProjectSource, error) {
	var source ProjectSource
	err := g.db.WithContext(ctx).Where("project_id = ? AND source_revision = ?", projectID, sourceRevision).
		Order("id DESC").First(&source).Error
	return source, err
}

func (g *GORMProjectSourceDAO) FindByID(ctx context.Context, id int64) (ProjectSource, error) {
	var source ProjectSource
	err := g.db.WithContext(ctx).Where("id = ?", id).First(&source).Error
	return source, err
}

func (g *GORMProjectSourceDAO) Create(ctx context.Context,
	source ProjectSource) (ProjectSource, error) {
	source.CTime = time.Now().UnixMilli()
	err := g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var project CodebookProject
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "source_revision").Where("id = ?", source.ProjectID).
			First(&project).Error; err != nil {
			return fmt.Errorf("锁定项目源码修订失败: %w", err)
		}
		if project.SourceRevision != source.SourceRevision {
			return fmt.Errorf("项目源码在归档上传期间发生变更，请重试")
		}
		var persisted ProjectSource
		if err := tx.Where("project_id = ? AND source_revision = ? AND digest = ?",
			source.ProjectID, source.SourceRevision, source.Digest).
			Attrs(source).FirstOrCreate(&persisted).Error; err != nil {
			return err
		}
		source = persisted
		return nil
	})
	return source, err
}
