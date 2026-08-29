package program_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	artifactarchive "github.com/Duke1616/etask/internal/artifact/archive"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	repositorymocks "github.com/Duke1616/etask/internal/repository/mocks"
	program "github.com/Duke1616/etask/internal/service/program"
	blobstoremocks "github.com/Duke1616/etask/pkg/blobstore/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gorm.io/gorm"
)

type codebooks struct{ values map[int64]domain.Codebook }

func (c codebooks) GetByID(_ context.Context, id int64) (domain.Codebook, error) {
	return c.values[id], nil
}

func TestSpecFromRunnerBinding(t *testing.T) {
	testCases := []struct {
		name       string
		codebookID int64
		kind       domain.ProgramKind
		want       *domain.ProgramSpec
		wantErr    string
	}{
		{
			name:       "inline",
			codebookID: 11,
			kind:       domain.ProgramInline,
			want:       &domain.ProgramSpec{Kind: domain.ProgramInline, Inline: &domain.InlineProgramSpec{CodebookID: 11}},
		},
		{
			name:       "project",
			codebookID: 12,
			kind:       domain.ProgramProject,
			want:       &domain.ProgramSpec{Kind: domain.ProgramProject, Project: &domain.ProjectProgramSpec{EntryCodebookID: 12}},
		},
		{name: "missing codebook", kind: domain.ProgramInline, wantErr: "未绑定程序来源"},
		{name: "invalid kind", codebookID: 11, kind: domain.ProgramKind("UNKNOWN"), wantErr: "程序类型非法"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := program.SpecFromRunnerBinding(testCase.codebookID, testCase.kind)
			if testCase.wantErr != "" {
				require.ErrorContains(t, err, testCase.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, testCase.want, got)
		})
	}
}

func TestResolveInline(t *testing.T) {
	testCases := []struct {
		name       string
		codebook   domain.Codebook
		wantCode   string
		wantProjID int64
		wantErr    string
	}{
		{
			name:       "普通文本代码文件成功",
			codebook:   domain.Codebook{ID: 11, ProjectID: 9, Kind: domain.CodebookKindFile, Code: "print('ok')"},
			wantCode:   "print('ok')",
			wantProjID: 9,
		},
		{
			name:     "拒绝Blob存储的大文件作为INLINE程序",
			codebook: domain.Codebook{ID: 11, ProjectID: 9, Kind: domain.CodebookKindFile, StorageType: domain.CodebookContentBlob},
			wantErr:  "PROJECT",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := program.NewService(codebooks{values: map[int64]domain.Codebook{
				11: tc.codebook,
			}}, nil, nil, nil)
			got, err := svc.Resolve(t.Context(), &domain.ProgramSpec{
				Kind: domain.ProgramInline, Inline: &domain.InlineProgramSpec{CodebookID: 11},
			})
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantCode, got.Program.Inline.Code)
			require.Equal(t, tc.wantProjID, got.SourceProjectID)
		})
	}
}

func TestResolveProject(t *testing.T) {
	validDigest := strings.Repeat("a", 64)
	validBlobChecksum := strings.Repeat("b", 64)

	testCases := []struct {
		name     string
		mock     func(ctrl *gomock.Controller, repo *repositorymocks.MockProjectSourceRepository, store *blobstoremocks.MockStore)
		nodes    map[int64]domain.Codebook
		validate func(t *testing.T, res program.Resolution, err error)
	}{
		{
			name: "成功_解析项目目录并复用源码快照",
			mock: func(ctrl *gomock.Controller, repo *repositorymocks.MockProjectSourceRepository, store *blobstoremocks.MockStore) {
				repo.EXPECT().GetProject(gomock.Any(), int64(9)).Return(domain.CodebookProject{
					ID: 9, Scope: domain.CodebookScopeTenant, Status: domain.CodebookProjectStatusArchived,
					SourceRevision: 4,
				}, nil)
				source := domain.ProjectSource{
					ID: 21, ProjectID: 9, SourceRevision: 4,
					Digest: validDigest, BlobChecksum: validBlobChecksum,
					Size: 128, Format: "tar.zst", FormatVersion: 1,
				}
				repo.EXPECT().FindByRevision(gomock.Any(), int64(9), int64(4)).Return(source, nil)
			},
			nodes: map[int64]domain.Codebook{
				11: {ID: 11, ProjectID: 9, ParentID: 10, Depth: 1, Name: "deploy.yml", Kind: domain.CodebookKindFile, Scope: domain.CodebookScopeTenant},
				10: {ID: 10, ProjectID: 9, Name: "playbooks", Kind: domain.CodebookKindDirectory, Scope: domain.CodebookScopeTenant},
			},
			validate: func(t *testing.T, res program.Resolution, err error) {
				require.NoError(t, err)
				require.Equal(t, "playbooks/deploy.yml", res.Program.Project.EntryPoint)
				require.Equal(t, int64(21), res.Program.Project.Source.SourceID)
			},
		},
		{
			name: "失败_读取项目源码文件出错",
			mock: func(ctrl *gomock.Controller, repo *repositorymocks.MockProjectSourceRepository, store *blobstoremocks.MockStore) {
				repo.EXPECT().GetProject(gomock.Any(), int64(9)).Return(domain.CodebookProject{ID: 9, Scope: domain.CodebookScopeTenant, SourceRevision: 1}, nil)
				repo.EXPECT().FindByRevision(gomock.Any(), int64(9), int64(1)).Return(domain.ProjectSource{}, gorm.ErrRecordNotFound)
				repo.EXPECT().SourceFiles(gomock.Any(), domain.ArtifactTarget{
					Scope: domain.CodebookScopeTenant, ProjectID: 9,
				}).Return(nil, int64(0), errors.New("read failed"))
			},
			nodes: map[int64]domain.Codebook{
				11: {ID: 11, ProjectID: 9, Name: "main.yml", Kind: domain.CodebookKindFile, Scope: domain.CodebookScopeTenant},
			},
			validate: func(t *testing.T, _ program.Resolution, err error) {
				require.ErrorContains(t, err, "read failed")
			},
		},
		{
			name: "失败_代码项目不存在报错Unavailable",
			mock: func(ctrl *gomock.Controller, repo *repositorymocks.MockProjectSourceRepository, store *blobstoremocks.MockStore) {
				repo.EXPECT().GetProject(gomock.Any(), int64(9)).Return(domain.CodebookProject{}, gorm.ErrRecordNotFound)
			},
			nodes: map[int64]domain.Codebook{
				11: {ID: 11, ProjectID: 9, Name: "main.yml", Kind: domain.CodebookKindFile, Scope: domain.CodebookScopeTenant},
			},
			validate: func(t *testing.T, _ program.Resolution, err error) {
				require.ErrorIs(t, err, errs.ErrProgramSourceUnavailable)
				require.ErrorContains(t, err, "不存在")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := repositorymocks.NewMockProjectSourceRepository(ctrl)
			store := blobstoremocks.NewMockStore(ctrl)
			tc.mock(ctrl, repo, store)
			svc := program.NewService(codebooks{values: tc.nodes}, repo, store, artifactarchive.New(""))
			res, err := svc.Resolve(ctxutil.WithTenantID(t.Context(), 10), &domain.ProgramSpec{
				Kind: domain.ProgramProject, Project: &domain.ProjectProgramSpec{EntryCodebookID: 11},
			})
			tc.validate(t, res, err)
		})
	}
}

func TestResolveProject_ConcurrentSingleflight(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymocks.NewMockProjectSourceRepository(ctrl)

	validDigest := strings.Repeat("a", 64)
	expectedSource := domain.ProjectSource{
		ID: 99, TenantID: 10, ProjectID: 9, SourceRevision: 3,
		Digest: validDigest, BlobChecksum: validDigest, Size: 1024, Format: "tar.zst", FormatVersion: 1,
	}

	started := make(chan struct{})
	release := make(chan struct{})

	// 10 个并发协程同时到达
	repo.EXPECT().GetProject(gomock.Any(), int64(9)).Return(domain.CodebookProject{
		ID: 9, Scope: domain.CodebookScopeTenant, SourceRevision: 3,
	}, nil).Times(10)

	// 核心验证：在耗时进行中，FindByRevision 只被 singleflight 执行 1 次！
	repo.EXPECT().FindByRevision(gomock.Any(), int64(9), int64(3)).DoAndReturn(
		func(ctx context.Context, projectID, revision int64) (domain.ProjectSource, error) {
			close(started) // 标志已进入单飞闭包
			<-release      // 阻塞住，模拟正在处理打包/查询
			return expectedSource, nil
		}).Times(1)

	svc := program.NewService(codebooks{values: map[int64]domain.Codebook{
		11: {ID: 11, ProjectID: 9, Name: "main.yml", Kind: domain.CodebookKindFile, Scope: domain.CodebookScopeTenant},
	}}, repo, blobstoremocks.NewMockStore(ctrl), artifactarchive.New(""))

	var wg sync.WaitGroup
	errsChan := make(chan error, 10)
	resultsChan := make(chan program.Resolution, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := svc.Resolve(ctxutil.WithTenantID(t.Context(), 10), &domain.ProgramSpec{
				Kind: domain.ProgramProject, Project: &domain.ProjectProgramSpec{EntryCodebookID: 11},
			})
			if err != nil {
				errsChan <- err
			} else {
				resultsChan <- res
			}
		}()
	}

	<-started                         // 确认第一个已进入并在执行中
	time.Sleep(10 * time.Millisecond) // 确保其余协程已到达并进入 singleflight 阻塞等待
	close(release)                    // 释放结果

	wg.Wait()
	close(errsChan)
	close(resultsChan)

	for err := range errsChan {
		require.NoError(t, err)
	}

	require.Equal(t, 10, len(resultsChan))
	for res := range resultsChan {
		require.Equal(t, int64(99), res.Program.Project.Source.SourceID)
		require.Equal(t, "main.yml", res.Program.Project.EntryPoint)
	}
}

func TestResolveProject_SingleflightContextDecoupled(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := repositorymocks.NewMockProjectSourceRepository(ctrl)

	cancelledCtx, cancel := context.WithCancel(ctxutil.WithTenantID(context.Background(), 10))
	cancel() // 立刻取消外部上下文

	repo.EXPECT().GetProject(gomock.Any(), int64(9)).Return(domain.CodebookProject{
		ID: 9, Scope: domain.CodebookScopeTenant, SourceRevision: 3,
	}, nil)

	// 核心验证：即使调用方传入的 Context 已经被 Cancel，由于使用了 WithoutCancel 脱钩，
	// 进入底层的 Context 依然活跃（ctx.Err() == nil），保证落盘与查询不受单点超时/取消破坏。
	repo.EXPECT().FindByRevision(gomock.Any(), int64(9), int64(3)).DoAndReturn(
		func(ctx context.Context, projectID, revision int64) (domain.ProjectSource, error) {
			require.NoError(t, ctx.Err(), "底层任务上下文应与外部取消脱钩")
			return domain.ProjectSource{
				ID: 99, TenantID: 10, ProjectID: 9, SourceRevision: 3,
				Digest: strings.Repeat("a", 64), BlobChecksum: strings.Repeat("a", 64),
				Size: 1024, Format: "tar.zst", FormatVersion: 1,
			}, nil
		})

	svc := program.NewService(codebooks{values: map[int64]domain.Codebook{
		11: {ID: 11, ProjectID: 9, Name: "main.yml", Kind: domain.CodebookKindFile, Scope: domain.CodebookScopeTenant},
	}}, repo, blobstoremocks.NewMockStore(ctrl), artifactarchive.New(""))

	res, err := svc.Resolve(cancelledCtx, &domain.ProgramSpec{
		Kind: domain.ProgramProject, Project: &domain.ProjectProgramSpec{EntryCodebookID: 11},
	})
	require.NoError(t, err)
	require.Equal(t, int64(99), res.Program.Project.Source.SourceID)
}
