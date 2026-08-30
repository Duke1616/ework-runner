package codebook_test

import (
	"context"
	"testing"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/etask/internal/domain"
	repositorymocks "github.com/Duke1616/etask/internal/repository/mocks"
	codebooksvc "github.com/Duke1616/etask/internal/service/codebook"
	blobstoremocks "github.com/Duke1616/etask/pkg/blobstore/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestProjectDeletionService_Preview(t *testing.T) {
	testCases := []struct {
		name      string
		ctx       context.Context
		projectID int64
		mock      func(repo *repositorymocks.MockProjectDeletionRepository)
		want      domain.ProjectDeleteImpact
		wantErr   string
	}{
		{
			name:      "失败_缺少租户上下文",
			ctx:       context.Background(),
			projectID: 7,
			mock:      func(repo *repositorymocks.MockProjectDeletionRepository) {},
			wantErr:   "项目删除参数非法",
		},
		{
			name:      "成功_查询项目删除影响",
			ctx:       ctxutil.WithTenantID(context.Background(), 10),
			projectID: 7,
			mock: func(repo *repositorymocks.MockProjectDeletionRepository) {
				repo.EXPECT().Preview(gomock.Any(), int64(7)).Return(domain.ProjectDeleteImpact{
					CodebookNodeCount: 3,
				}, nil)
			},
			want: domain.ProjectDeleteImpact{CodebookNodeCount: 3},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := repositorymocks.NewMockProjectDeletionRepository(ctrl)
			store := blobstoremocks.NewMockStore(ctrl)
			tc.mock(repo)

			service := codebooksvc.NewProjectDeletionService(repo, store)
			got, err := service.Preview(tc.ctx, tc.projectID)
			if tc.wantErr != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestProjectDeletionService_Delete(t *testing.T) {
	testCases := []struct {
		name        string
		ctx         context.Context
		projectID   int64
		projectName string
		mock        func(repo *repositorymocks.MockProjectDeletionRepository, store *blobstoremocks.MockStore)
		wantErr     string
	}{
		{
			name:        "失败_缺少租户上下文",
			ctx:         context.Background(),
			projectID:   7,
			projectName: "剧本",
			mock:        func(repo *repositorymocks.MockProjectDeletionRepository, store *blobstoremocks.MockStore) {},
			wantErr:     "项目删除参数非法",
		},
		{
			name:        "成功_提交删除并去重清理对象存储",
			ctx:         ctxutil.WithTenantID(context.Background(), 99),
			projectID:   7,
			projectName: "剧本",
			mock: func(repo *repositorymocks.MockProjectDeletionRepository, store *blobstoremocks.MockStore) {
				repo.EXPECT().Delete(gomock.Any(), int64(7), "剧本").Return(domain.ProjectDeleteCleanup{
					ObjectKeys: []string{"one", "one", "two"},
				}, nil)
				store.EXPECT().Delete(gomock.Any(), "one").Return(nil)
				store.EXPECT().Delete(gomock.Any(), "two").Return(nil)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := repositorymocks.NewMockProjectDeletionRepository(ctrl)
			store := blobstoremocks.NewMockStore(ctrl)
			tc.mock(repo, store)

			service := codebooksvc.NewProjectDeletionService(repo, store)
			err := service.Delete(tc.ctx, tc.projectID, tc.projectName)
			if tc.wantErr != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}
