package migrations

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/Duke1616/eiam/pkg/migration"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository/dao"
	"github.com/Duke1616/etask/pkg/sorter"
)

// mongoCodebook 是 eflow MongoDB 中的脚本模板源数据。
type mongoCodebook struct {
	ID         int64  `bson:"id"`
	Name       string `bson:"name"`
	Owner      string `bson:"owner"`
	Identifier string `bson:"identifier"`
	Code       string `bson:"code"`
	Language   string `bson:"language"`
	Secret     string `bson:"secret"`
	CTime      int64  `bson:"ctime"`
	UTime      int64  `bson:"utime"`
}

type codebookMigrator struct{}

// NewCodebookMigrator 创建脚本模板迁移器。
func NewCodebookMigrator() migration.Migrator {
	return migration.NewMongoMigrator[mongoCodebook, dao.Codebook](codebookMigrator{})
}

func (codebookMigrator) Name() string {
	return "codebook"
}

func (codebookMigrator) CollectionName() string {
	return "c_codebook"
}

func (codebookMigrator) Convert(src mongoCodebook) dao.Codebook {
	return dao.Codebook{
		ID:               src.ID,
		TenantID:         DefaultTenantID,
		Scope:            domain.CodebookScopeTenant.String(),
		ProjectID:        1,
		Name:             codebookFileName(src),
		Owner:            src.Owner,
		Kind:             domain.CodebookKindFile.String(),
		SortNo:           src.ID * sorter.DefaultIndexGap,
		Secret:           src.Secret,
		CurrentVersionID: src.ID,
		CTime:            src.CTime,
		UTime:            src.UTime,
	}
}

type codebookVersionMigrator struct{}

// NewCodebookVersionMigrator 创建脚本模板版本迁移器。
func NewCodebookVersionMigrator() migration.Migrator {
	return migration.NewMongoMigrator[mongoCodebook, dao.CodebookVersion](codebookVersionMigrator{})
}

func (codebookVersionMigrator) Name() string {
	return "codebook_version"
}

func (codebookVersionMigrator) CollectionName() string {
	return "c_codebook"
}

func (codebookVersionMigrator) Convert(src mongoCodebook) dao.CodebookVersion {
	return dao.CodebookVersion{
		ID:          src.ID,
		NodeID:      src.ID,
		TenantID:    DefaultTenantID,
		Scope:       domain.CodebookScopeTenant.String(),
		VersionNo:   1,
		Code:        src.Code,
		StorageType: domain.CodebookContentInline.String(),
		Size:        int64(len(src.Code)),
		ContentType: "text/plain; charset=utf-8",
		Hash:        hashCodebookCode(src.Code),
		CTime:       src.CTime,
	}
}

func codebookFileName(src mongoCodebook) string {
	name := strings.TrimSpace(src.Identifier)
	ext := codebookFileExt(src.Language)
	if ext != "" && !strings.HasSuffix(strings.ToLower(name), ext) {
		name += ext
	}
	return name
}

func codebookFileExt(language string) string {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "shell", "sh", "bash":
		return ".sh"
	case "python", "py":
		return ".py"
	default:
		return ""
	}
}

func hashCodebookCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}
