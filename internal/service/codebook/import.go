package codebook

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	artifactarchive "github.com/Duke1616/etask/internal/artifact/archive"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	"github.com/Duke1616/etask/pkg/blobstore"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

//go:generate go tool mockgen -source=./import.go -package=codebookmocks -destination=./mocks/project_file_repository.mock.go -typed

const (
	inlineContentMaxSize = int64(64 << 10)
	importMaxFileSize    = int64(512 << 20)
	importMaxTotalSize   = int64(1 << 30)
	importMaxFileCount   = 10000
	importMaxDepth       = 64
)

// ImportFile 描述一个待导入文件及其项目内相对路径。
type ImportFile struct {
	Path        string
	Size        int64
	ContentType string
	Open        func() (io.ReadCloser, error)
}

// ImportRequest 描述一次原子项目文件导入。
type ImportRequest struct {
	ProjectID int64
	ParentID  int64
	Files     []ImportFile
}

// ProjectFileRepository 是项目文件服务依赖的最小 Codebook 仓储能力。
type ProjectFileRepository interface {
	// GetProjectByID 查询导入目标项目。
	GetProjectByID(ctx context.Context, id int64) (domain.CodebookProject, error)
	// GetNodeByID 查询导入目标目录元信息。
	GetNodeByID(ctx context.Context, id int64) (domain.Codebook, error)
	// GetByID 查询文件及其当前版本内容元数据。
	GetByID(ctx context.Context, id int64) (domain.Codebook, error)
	// Import 原子写入已经完成内容持久化的项目文件树。
	Import(ctx context.Context, request domain.CodebookImport) (domain.CodebookImportResult, error)
	// Delete 原子删除节点子树，并返回需要清理的外部内容对象。
	Delete(ctx context.Context, id int64) (domain.CodebookDeleteResult, error)
}

// ProjectFileService 提供项目文件导入和内容读取能力。
type ProjectFileService interface {
	// Import 校验并原子导入文件树。
	Import(ctx context.Context, request ImportRequest) (domain.CodebookImportResult, error)
	// Open 打开项目文件当前版本的内容流。
	Open(ctx context.Context, nodeID int64) (domain.Codebook, io.ReadCloser, error)
	// Delete 删除节点子树及其 Blob 内容。
	Delete(ctx context.Context, nodeID int64) (int64, error)
}

type projectFileService struct {
	repo  ProjectFileRepository
	store blobstore.Store
}

// NewProjectFileService 创建 Codebook 项目文件服务。
func NewProjectFileService(repo ProjectFileRepository, store blobstore.Store) ProjectFileService {
	return &projectFileService{repo: repo, store: store}
}

func (s *projectFileService) Import(ctx context.Context,
	request ImportRequest) (domain.CodebookImportResult, error) {
	if request.ProjectID <= 0 {
		return domain.CodebookImportResult{}, fmt.Errorf("%w: 项目 ID 非法", errs.ErrInvalidParameter)
	}
	if len(request.Files) == 0 || len(request.Files) > importMaxFileCount {
		return domain.CodebookImportResult{}, fmt.Errorf("%w: 导入文件数量必须在 1 到 %d 之间",
			errs.ErrInvalidParameter, importMaxFileCount)
	}
	project, err := s.repo.GetProjectByID(ctx, request.ProjectID)
	if err != nil {
		return domain.CodebookImportResult{}, err
	}
	if project.Scope != domain.CodebookScopeTenant {
		return domain.CodebookImportResult{}, fmt.Errorf("%w: 只能向租户项目导入文件", errs.ErrInvalidParameter)
	}
	if err = project.ValidateWritable(); err != nil {
		return domain.CodebookImportResult{}, err
	}
	if err = validateCodebookWriteScope(ctx, project.Scope); err != nil {
		return domain.CodebookImportResult{}, err
	}
	if request.ParentID > 0 {
		parent, parentErr := s.repo.GetNodeByID(ctx, request.ParentID)
		if parentErr != nil {
			return domain.CodebookImportResult{}, parentErr
		}
		if !parent.IsDirectory() || parent.ProjectID != request.ProjectID {
			return domain.CodebookImportResult{}, fmt.Errorf("%w: 导入目标不是当前项目目录", errs.ErrInvalidParameter)
		}
	}

	totalSize := int64(0)
	seen := make(map[string]struct{}, len(request.Files))
	for index := range request.Files {
		file := &request.Files[index]
		file.Path, err = validateImportPath(file.Path)
		if err != nil {
			return domain.CodebookImportResult{}, err
		}
		key := strings.ToLower(file.Path)
		if _, exists := seen[key]; exists {
			return domain.CodebookImportResult{}, fmt.Errorf("%w: 导入内容包含重复路径 %s",
				errs.ErrInvalidParameter, file.Path)
		}
		seen[key] = struct{}{}
		if file.Size < 0 || file.Size > importMaxFileSize {
			return domain.CodebookImportResult{}, fmt.Errorf("%w: 文件 %s 大小超出限制",
				errs.ErrInvalidParameter, file.Path)
		}
		totalSize += file.Size
		if totalSize > importMaxTotalSize {
			return domain.CodebookImportResult{}, fmt.Errorf("%w: 导入文件总大小超出限制",
				errs.ErrInvalidParameter)
		}
	}

	tenantID := ctxutil.GetTenantID(ctx).Int64()
	prepared := make([]domain.CodebookImportFile, 0, len(request.Files))
	createdObjects := make([]string, 0)
	for _, file := range request.Files {
		if file.Open == nil {
			s.cleanup(ctx, createdObjects)
			return domain.CodebookImportResult{}, fmt.Errorf("%w: 文件 %s 缺少内容", errs.ErrInvalidParameter, file.Path)
		}
		reader, openErr := file.Open()
		if openErr != nil {
			s.cleanup(ctx, createdObjects)
			return domain.CodebookImportResult{}, fmt.Errorf("打开文件 %s 失败: %w", file.Path, openErr)
		}
		content, contentErr := s.prepareContent(ctx, tenantID, file, reader)
		closeErr := reader.Close()
		if contentErr != nil {
			s.cleanup(ctx, createdObjects)
			return domain.CodebookImportResult{}, contentErr
		}
		if closeErr != nil {
			s.cleanup(ctx, createdObjects)
			return domain.CodebookImportResult{}, fmt.Errorf("关闭文件 %s 失败: %w", file.Path, closeErr)
		}
		prepared = append(prepared, content)
		if content.StorageType == domain.CodebookContentBlob {
			createdObjects = append(createdObjects, content.ObjectKey)
		}
	}

	result, err := s.repo.Import(ctx, domain.CodebookImport{
		ProjectID: request.ProjectID, ParentID: request.ParentID, Files: prepared,
	})
	if err != nil {
		s.cleanup(ctx, createdObjects)
		return domain.CodebookImportResult{}, err
	}
	return result, nil
}

func (s *projectFileService) prepareContent(ctx context.Context, tenantID int64,
	file ImportFile, reader io.Reader) (domain.CodebookImportFile, error) {
	if file.Size <= inlineContentMaxSize {
		content, err := io.ReadAll(io.LimitReader(reader, file.Size+1))
		if err != nil {
			return domain.CodebookImportFile{}, fmt.Errorf("读取文件 %s 失败: %w", file.Path, err)
		}
		if int64(len(content)) != file.Size {
			return domain.CodebookImportFile{}, fmt.Errorf("%w: 文件 %s 大小不一致", errs.ErrInvalidParameter, file.Path)
		}
		if isEditableText(content) {
			hash := sha256.Sum256(content)
			return domain.CodebookImportFile{
				Path: file.Path, Code: string(content), StorageType: domain.CodebookContentInline,
				Size: file.Size, ContentType: detectContentType(file.Path, file.ContentType, content),
				Hash: hex.EncodeToString(hash[:]),
			}, nil
		}
		return s.putBlob(ctx, tenantID, file, bytes.NewReader(content), content)
	}
	return s.putBlob(ctx, tenantID, file, reader, nil)
}

func (s *projectFileService) putBlob(ctx context.Context, tenantID int64, file ImportFile,
	reader io.Reader, sample []byte) (domain.CodebookImportFile, error) {
	contentType := detectContentType(file.Path, file.ContentType, sample)
	objectKey := fmt.Sprintf("codebook-content/%d/%s", tenantID, uuid.NewString())
	hash := sha256.New()
	if err := s.store.Put(ctx, objectKey, io.TeeReader(reader, hash), blobstore.PutOptions{
		Size: file.Size, ContentType: contentType,
	}); err != nil {
		return domain.CodebookImportFile{}, fmt.Errorf("保存文件 %s 失败: %w", file.Path, err)
	}
	return domain.CodebookImportFile{
		Path: file.Path, StorageType: domain.CodebookContentBlob, ObjectKey: objectKey,
		Size: file.Size, ContentType: contentType, Hash: hex.EncodeToString(hash.Sum(nil)),
	}, nil
}

func (s *projectFileService) cleanup(ctx context.Context, keys []string) {
	for _, key := range keys {
		_ = s.store.Delete(ctx, key)
	}
}

func (s *projectFileService) Open(ctx context.Context,
	nodeID int64) (domain.Codebook, io.ReadCloser, error) {
	if nodeID <= 0 {
		return domain.Codebook{}, nil, fmt.Errorf("%w: 文件 ID 非法", errs.ErrInvalidParameter)
	}
	file, err := s.repo.GetByID(ctx, nodeID)
	if err != nil {
		return domain.Codebook{}, nil, err
	}
	if !file.IsFile() {
		return domain.Codebook{}, nil, fmt.Errorf("%w: 目录没有文件内容", errs.ErrInvalidParameter)
	}
	if file.StorageType == domain.CodebookContentBlob {
		reader, openErr := s.store.Open(ctx, file.ObjectKey)
		return file, reader, openErr
	}
	return file, io.NopCloser(strings.NewReader(file.Code)), nil
}

// Delete 删除节点子树，并在数据库事务提交后清理对应的 Blob 内容。
func (s *projectFileService) Delete(ctx context.Context, nodeID int64) (int64, error) {
	if nodeID <= 0 {
		return 0, fmt.Errorf("%w: 文件 ID 非法", errs.ErrInvalidParameter)
	}
	node, err := s.repo.GetNodeByID(ctx, nodeID)
	if err != nil {
		return 0, err
	}
	if err = validateCodebookWriteScope(ctx, node.Scope); err != nil {
		return 0, err
	}
	if node.Scope == domain.CodebookScopeTenant {
		project, projectErr := s.repo.GetProjectByID(ctx, node.ProjectID)
		if projectErr != nil {
			return 0, projectErr
		}
		if projectErr = project.ValidateWritable(); projectErr != nil {
			return 0, projectErr
		}
	}
	result, err := s.repo.Delete(ctx, nodeID)
	if err != nil {
		return 0, err
	}
	if err = s.deleteObjects(context.WithoutCancel(ctx), result.ObjectKeys); err != nil {
		return result.NodeCount, err
	}
	return result.NodeCount, nil
}

func (s *projectFileService) deleteObjects(ctx context.Context, keys []string) error {
	var group errgroup.Group
	group.SetLimit(8)
	for _, key := range keys {
		key := key
		group.Go(func() error {
			if err := s.store.Delete(ctx, key); err != nil {
				return fmt.Errorf("删除文件内容 %s 失败: %w", key, err)
			}
			return nil
		})
	}
	return group.Wait()
}

func validateImportPath(value string) (string, error) {
	clean, err := artifactarchive.ValidatePath(strings.TrimSpace(value))
	if err != nil {
		return "", fmt.Errorf("%w: %v", errs.ErrInvalidParameter, err)
	}
	segments := strings.Split(clean, "/")
	if len(segments) > importMaxDepth {
		return "", fmt.Errorf("%w: 文件路径层级超出限制: %s", errs.ErrInvalidParameter, clean)
	}
	for _, segment := range segments {
		if len(segment) > 128 || strings.ContainsRune(segment, '\x00') {
			return "", fmt.Errorf("%w: 文件路径节点非法: %s", errs.ErrInvalidParameter, segment)
		}
	}
	return clean, nil
}

func isEditableText(content []byte) bool {
	return utf8.Valid(content) && !bytes.ContainsRune(content, '\x00')
}

func detectContentType(name, declared string, sample []byte) string {
	if value, _, err := mime.ParseMediaType(strings.TrimSpace(declared)); err == nil && value != "" {
		return value
	}
	if value := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); value != "" {
		return value
	}
	if len(sample) > 0 {
		return http.DetectContentType(sample)
	}
	return "application/octet-stream"
}
