package domain

import (
	"encoding/hex"
	"fmt"
	"path"
	"strings"

	"github.com/Duke1616/etask/internal/errs"
)

// ProgramKind 描述程序以单文件还是完整项目运行。
type ProgramKind string

const (
	ProgramInline  ProgramKind = "INLINE"
	ProgramProject ProgramKind = "PROJECT"
)

func (k ProgramKind) Valid() bool { return k == ProgramInline || k == ProgramProject }

// ProgramSpec 是任务中由用户声明的程序配置，不包含运行时解析结果。
type ProgramSpec struct {
	Kind    ProgramKind         `json:"kind"`
	Inline  *InlineProgramSpec  `json:"inline,omitempty"`
	Project *ProjectProgramSpec `json:"project,omitempty"`
}

// InlineProgramSpec 支持直接代码或 Codebook 单文件引用，两者必须且只能指定一个。
type InlineProgramSpec struct {
	Code       string `json:"code,omitempty"`
	CodebookID int64  `json:"codebookId,omitempty"`
}

// ProjectProgramSpec 只声明项目中的入口文件，项目源码由系统解析。
type ProjectProgramSpec struct {
	EntryCodebookID int64 `json:"entryCodebookId"`
}

func (s ProgramSpec) Validate() error {
	if !s.Kind.Valid() {
		return fmt.Errorf("%w: 不支持的程序类型: %s", errs.ErrInvalidParameter, s.Kind)
	}
	switch s.Kind {
	case ProgramInline:
		if s.Inline == nil || s.Project != nil {
			return fmt.Errorf("%w: INLINE 程序配置非法", errs.ErrInvalidParameter)
		}
		hasCode, hasCodebook := s.Inline.Code != "", s.Inline.CodebookID > 0
		if hasCode == hasCodebook {
			return fmt.Errorf("%w: INLINE 必须且只能指定 code 或 codebook_id", errs.ErrInvalidParameter)
		}
	case ProgramProject:
		if s.Project == nil || s.Inline != nil || s.Project.EntryCodebookID <= 0 {
			return fmt.Errorf("%w: PROJECT 必须指定有效的入口 Codebook 文件", errs.ErrInvalidParameter)
		}
	}
	return nil
}

// Program 是固定在任务执行记录中的不可变程序输入。
type Program struct {
	Kind    ProgramKind     `json:"kind"`
	Inline  *InlineProgram  `json:"inline,omitempty"`
	Project *ProjectProgram `json:"project,omitempty"`
}

type InlineProgram struct {
	Code string `json:"code"`
}

type ProjectProgram struct {
	Source     ProjectSourceRef `json:"source"`
	EntryPoint string           `json:"entryPoint"`
}

// ProjectSourceRef 固定一次 PROJECT 执行使用的项目源码修订。
type ProjectSourceRef struct {
	SourceID       int64  `json:"sourceId"`
	ProjectID      int64  `json:"projectId"`
	SourceRevision int64  `json:"sourceRevision"`
	Digest         string `json:"digest"`
	BlobChecksum   string `json:"blobChecksum"`
	Size           int64  `json:"size"`
	Format         string `json:"format"`
	FormatVersion  int32  `json:"formatVersion"`
}

// ProjectSource 是系统自动生成的不可变项目源码记录。
type ProjectSource struct {
	ID             int64
	TenantID       int64
	ProjectID      int64
	SourceRevision int64
	Digest         string
	BlobChecksum   string
	ObjectKey      string
	Size           int64
	Format         string
	FormatVersion  int32
	CTime          int64
}

func (s ProjectSource) Ref() ProjectSourceRef {
	return ProjectSourceRef{
		SourceID: s.ID, ProjectID: s.ProjectID, SourceRevision: s.SourceRevision,
		Digest: s.Digest, BlobChecksum: s.BlobChecksum, Size: s.Size,
		Format: s.Format, FormatVersion: s.FormatVersion,
	}
}

func NewInlineProgram(code string) *Program {
	return &Program{Kind: ProgramInline, Inline: &InlineProgram{Code: code}}
}

func (s Program) Validate() error {
	if !s.Kind.Valid() {
		return fmt.Errorf("%w: 不支持的程序类型: %s", errs.ErrInvalidParameter, s.Kind)
	}
	switch s.Kind {
	case ProgramInline:
		if s.Inline == nil || s.Project != nil || s.Inline.Code == "" {
			return fmt.Errorf("%w: INLINE 程序非法", errs.ErrInvalidParameter)
		}
	case ProgramProject:
		if s.Project == nil || s.Inline != nil {
			return fmt.Errorf("%w: PROJECT 程序非法", errs.ErrInvalidParameter)
		}
		if err := s.Project.Source.Validate(); err != nil {
			return fmt.Errorf("PROJECT 项目源码非法: %w", err)
		}
		if err := validateProjectEntryPoint(s.Project.EntryPoint); err != nil {
			return err
		}
	}
	return nil
}

func (r ProjectSourceRef) Validate() error {
	if r.SourceID <= 0 || r.ProjectID <= 0 || r.SourceRevision < 0 {
		return fmt.Errorf("%w: 项目源码标识非法", errs.ErrInvalidParameter)
	}
	if !validSourceDigest(r.Digest) || !validSourceDigest(r.BlobChecksum) {
		return fmt.Errorf("%w: 项目源码摘要非法", errs.ErrInvalidParameter)
	}
	if r.Size <= 0 || strings.TrimSpace(r.Format) == "" || r.FormatVersion <= 0 {
		return fmt.Errorf("%w: 项目源码格式或大小非法", errs.ErrInvalidParameter)
	}
	return nil
}

func validSourceDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validateProjectEntryPoint(value string) error {
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\\\x00") ||
		strings.HasPrefix(value, "/") || path.Clean(value) != value || value == "." || value == ".." ||
		strings.HasPrefix(value, "../") {
		return fmt.Errorf("%w: PROJECT 程序入口路径非法: %q", errs.ErrInvalidParameter, value)
	}
	return nil
}
