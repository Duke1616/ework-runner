package codebook

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository"
	"github.com/Duke1616/etask/pkg/blobstore"
	"github.com/stretchr/testify/require"
)

type projectDeletionRepositoryStub struct {
	impact      domain.ProjectDeleteImpact
	cleanup     domain.ProjectDeleteCleanup
	projectID   int64
	projectName string
}

func (s *projectDeletionRepositoryStub) Preview(_ context.Context,
	projectID int64) (domain.ProjectDeleteImpact, error) {
	s.projectID = projectID
	return s.impact, nil
}

func (s *projectDeletionRepositoryStub) Delete(_ context.Context,
	projectID int64, projectName string) (domain.ProjectDeleteCleanup, error) {
	s.projectID, s.projectName = projectID, projectName
	return s.cleanup, nil
}

type projectDeletionStoreStub struct {
	mu      sync.Mutex
	deleted []string
}

func (s *projectDeletionStoreStub) Put(context.Context, string, io.Reader, blobstore.PutOptions) error {
	return nil
}
func (s *projectDeletionStoreStub) Open(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (s *projectDeletionStoreStub) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleted = append(s.deleted, key)
	return nil
}

func TestProjectDeletionServiceValidatesTenantContext(t *testing.T) {
	service := NewProjectDeletionService(&projectDeletionRepositoryStub{}, &projectDeletionStoreStub{})

	_, err := service.Preview(t.Context(), 7)

	require.Error(t, err)
}

func TestProjectDeletionServiceDeletesObjectsAfterRepositoryCommit(t *testing.T) {
	repo := &projectDeletionRepositoryStub{cleanup: domain.ProjectDeleteCleanup{
		ObjectKeys: []string{"one", "one", "two"},
	}}
	store := &projectDeletionStoreStub{}
	service := NewProjectDeletionService(repo, store)
	ctx := ctxutil.WithTenantID(t.Context(), 99)

	err := service.Delete(ctx, 7, "剧本")

	require.NoError(t, err)
	require.Equal(t, int64(7), repo.projectID)
	require.Equal(t, "剧本", repo.projectName)
	require.ElementsMatch(t, []string{"one", "two"}, store.deleted)
}

var _ repository.ProjectDeletionRepository = (*projectDeletionRepositoryStub)(nil)
