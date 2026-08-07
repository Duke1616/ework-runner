package program

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	artifactarchive "github.com/Duke1616/etask/internal/artifact/archive"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository"
	"github.com/Duke1616/etask/pkg/blobstore"
	"gorm.io/gorm"
)

//go:generate go tool mockgen -source=./service.go -package=programmocks -destination=./mocks/service.mock.go -typed

// CodebookReader 提供解析程序声明所需的最小 Codebook 读取能力。
type CodebookReader interface {
	// GetByID 查询指定 Codebook 节点。
	GetByID(ctx context.Context, id int64) (domain.Codebook, error)
}

// Resolution 是用户程序声明解析后的执行输入。
type Resolution struct {
	Program         *domain.Program
	SourceProjectID int64
}

// Service 解析程序声明，并管理 PROJECT 模式使用的不可变项目源码。
type Service interface {
	// Resolve 将用户程序声明解析为不可变执行输入。
	Resolve(ctx context.Context, spec *domain.ProgramSpec) (Resolution, error)
	// OpenSource 打开校验通过的不可变项目源码内容。
	OpenSource(ctx context.Context, sourceID int64, digest string) (io.ReadCloser, error)
}

type modeResolver interface {
	// Resolve 解析当前模式的程序声明。
	Resolve(context.Context, *domain.ProgramSpec) (Resolution, error)
}

type service struct {
	codebooks CodebookReader
	repo      repository.ProjectSourceRepository
	store     blobstore.Store
	archive   *artifactarchive.Codec
	modes     map[domain.ProgramKind]modeResolver
}

func NewService(codebooks CodebookReader, repo repository.ProjectSourceRepository,
	store blobstore.Store, archive *artifactarchive.Codec) Service {
	svc := &service{codebooks: codebooks, repo: repo, store: store, archive: archive}
	svc.modes = map[domain.ProgramKind]modeResolver{
		domain.ProgramInline:  inlineResolver{codebooks: codebooks},
		domain.ProgramProject: projectResolver{service: svc},
	}
	return svc
}

func (s *service) Resolve(ctx context.Context, spec *domain.ProgramSpec) (Resolution, error) {
	if spec == nil {
		return Resolution{}, nil
	}
	if err := spec.Validate(); err != nil {
		return Resolution{}, fmt.Errorf("程序配置非法: %w", err)
	}
	resolver := s.modes[spec.Kind]
	return resolver.Resolve(ctx, spec)
}

type inlineResolver struct{ codebooks CodebookReader }

func (r inlineResolver) Resolve(ctx context.Context, spec *domain.ProgramSpec) (Resolution, error) {
	if spec.Inline.Code != "" {
		return Resolution{Program: domain.NewInlineProgram(spec.Inline.Code)}, nil
	}
	codebook, err := loadFile(ctx, r.codebooks, spec.Inline.CodebookID)
	if err != nil {
		return Resolution{}, err
	}
	return Resolution{Program: domain.NewInlineProgram(codebook.Code), SourceProjectID: codebook.ProjectID}, nil
}

type projectResolver struct{ service *service }

func (r projectResolver) Resolve(ctx context.Context, spec *domain.ProgramSpec) (Resolution, error) {
	s := r.service
	codebook, err := loadFile(ctx, s.codebooks, spec.Project.EntryCodebookID)
	if err != nil {
		return Resolution{}, err
	}
	entryPoint, err := s.entryPoint(ctx, codebook)
	if err != nil {
		return Resolution{}, err
	}
	source, err := s.prepareSource(ctx, codebook.ProjectID)
	if err != nil {
		return Resolution{}, fmt.Errorf("准备 PROJECT 项目源码失败: %w", err)
	}
	result := Resolution{SourceProjectID: codebook.ProjectID, Program: &domain.Program{
		Kind: domain.ProgramProject,
		Project: &domain.ProjectProgram{
			Source: source.Ref(), EntryPoint: entryPoint,
		},
	}}
	if err = result.Program.Validate(); err != nil {
		return Resolution{}, fmt.Errorf("生成 PROJECT 程序失败: %w", err)
	}
	return result, nil
}

func loadFile(ctx context.Context, reader CodebookReader, id int64) (domain.Codebook, error) {
	codebook, err := reader.GetByID(ctx, id)
	if err != nil {
		return domain.Codebook{}, fmt.Errorf("查询程序 Codebook 失败: %w", err)
	}
	if !codebook.IsFile() {
		return domain.Codebook{}, fmt.Errorf("程序入口必须绑定 Codebook 文件")
	}
	return codebook, nil
}

func (s *service) entryPoint(ctx context.Context, node domain.Codebook) (string, error) {
	segments := make([]string, 0, node.Depth+1)
	visited := make(map[int64]struct{}, node.Depth+1)
	projectID, scope := node.ProjectID, node.Scope
	for {
		if _, exists := visited[node.ID]; exists {
			return "", fmt.Errorf("Codebook 程序入口目录存在循环引用，节点 ID=%d", node.ID)
		}
		visited[node.ID] = struct{}{}
		if node.ProjectID != projectID || node.Scope != scope {
			return "", fmt.Errorf("Codebook 程序入口父级不属于同一项目")
		}
		if strings.TrimSpace(node.Name) == "" || node.Name == "." || node.Name == ".." ||
			strings.ContainsAny(node.Name, "/\\\x00") {
			return "", fmt.Errorf("Codebook 程序入口包含非法路径节点: %q", node.Name)
		}
		segments = append(segments, node.Name)
		if node.ParentID == 0 {
			break
		}
		parent, err := s.codebooks.GetByID(ctx, node.ParentID)
		if err != nil {
			return "", fmt.Errorf("查询 Codebook 程序入口父级失败: %w", err)
		}
		if !parent.IsDirectory() {
			return "", fmt.Errorf("Codebook 程序入口父级不是目录: %s", parent.Name)
		}
		node = parent
	}
	for left, right := 0, len(segments)-1; left < right; left, right = left+1, right-1 {
		segments[left], segments[right] = segments[right], segments[left]
	}
	return path.Join(segments...), nil
}

func (s *service) prepareSource(ctx context.Context, projectID int64) (domain.ProjectSource, error) {
	tenantID := ctxutil.GetTenantID(ctx).Int64()
	target := domain.ArtifactTarget{Scope: domain.CodebookScopeTenant, ProjectID: projectID}
	if err := target.ValidateWriteAccess(tenantID, ctxutil.SystemTenantID); err != nil {
		return domain.ProjectSource{}, err
	}
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return domain.ProjectSource{}, fmt.Errorf("查询代码项目失败: %w", err)
	}
	if project.ID != projectID || project.Scope != domain.CodebookScopeTenant {
		return domain.ProjectSource{}, fmt.Errorf("代码项目与 PROJECT 来源不匹配")
	}
	current, err := s.repo.FindByRevision(ctx, projectID, project.SourceRevision)
	if err == nil {
		return current, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ProjectSource{}, fmt.Errorf("查询项目源码失败: %w", err)
	}
	files, sourceRevision, err := s.repo.SourceFiles(ctx, target)
	if err != nil {
		return domain.ProjectSource{}, err
	}
	packed, err := s.archive.Pack(files)
	if err != nil {
		return domain.ProjectSource{}, err
	}
	defer os.Remove(packed.Path)
	file, err := os.Open(packed.Path)
	if err != nil {
		return domain.ProjectSource{}, fmt.Errorf("打开项目源码归档失败: %w", err)
	}
	defer file.Close()
	objectKey := fmt.Sprintf("project-sources/%d/%d/%d/%s.%s", tenantID, projectID,
		sourceRevision, packed.Digest, packed.Format)
	if err = s.store.Put(ctx, objectKey, file, packed.Size, packed.BlobChecksum); err != nil {
		return domain.ProjectSource{}, fmt.Errorf("保存项目源码失败: %w", err)
	}
	return s.repo.Create(ctx, domain.ProjectSource{
		TenantID: tenantID, ProjectID: projectID, SourceRevision: sourceRevision,
		Digest: packed.Digest, BlobChecksum: packed.BlobChecksum, ObjectKey: objectKey,
		Size: packed.Size, Format: packed.Format, FormatVersion: packed.FormatVersion,
	})
}

func (s *service) OpenSource(ctx context.Context, sourceID int64, digest string) (io.ReadCloser, error) {
	if sourceID <= 0 || !validDigest(digest) {
		return nil, blobstore.ErrNotFound
	}
	source, err := s.repo.FindByID(ctx, sourceID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, blobstore.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询项目源码失败: %w", err)
	}
	if source.Digest != digest {
		return nil, blobstore.ErrNotFound
	}
	reader, err := s.store.Open(ctx, source.ObjectKey)
	if errors.Is(err, blobstore.ErrNotFound) {
		return nil, blobstore.ErrNotFound
	}
	return reader, err
}

func validDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
