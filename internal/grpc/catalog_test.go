package grpc

import (
	"errors"
	"testing"

	codebookv1 "github.com/Duke1616/etask/api/proto/gen/etask/codebook/v1"
	runnerv1 "github.com/Duke1616/etask/api/proto/gen/etask/runner/v1"
	"github.com/Duke1616/etask/internal/domain"
	codebookmocks "github.com/Duke1616/etask/internal/service/codebook/mocks"
	runnermocks "github.com/Duke1616/etask/internal/service/runner/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestCodebookServer_GetCodebookByID(t *testing.T) {
	testCases := []struct {
		name        string
		codebookID  int64
		mock        func(ctrl *gomock.Controller) *codebookmocks.MockService
		wantErrCode codes.Code
		wantName    string
	}{
		{
			name:       "成功查询脚本模板",
			codebookID: 10,
			mock: func(ctrl *gomock.Controller) *codebookmocks.MockService {
				svc := codebookmocks.NewMockService(ctrl)
				svc.EXPECT().GetByID(gomock.Any(), int64(10)).Return(domain.Codebook{
					ID: 10, Name: "deploy.sh", Code: "echo ok",
				}, nil)
				return svc
			},
			wantName: "deploy.sh",
		},
		{
			name:       "脚本模板不存在返回NotFound",
			codebookID: 404,
			mock: func(ctrl *gomock.Controller) *codebookmocks.MockService {
				svc := codebookmocks.NewMockService(ctrl)
				svc.EXPECT().GetByID(gomock.Any(), int64(404)).Return(domain.Codebook{}, errors.New("not found"))
				return svc
			},
			wantErrCode: codes.NotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			svc := tc.mock(ctrl)
			server := NewCodebookServer(svc)

			res, err := server.GetCodebookByID(t.Context(), &codebookv1.GetCodebookByIDRequest{
				Id: tc.codebookID,
			})

			if tc.wantErrCode != codes.OK {
				require.Error(t, err)
				require.Equal(t, tc.wantErrCode, status.Code(err))
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.codebookID, res.GetCodebook().GetId())
			require.Equal(t, tc.wantName, res.GetCodebook().GetName())
		})
	}
}

func TestRunnerServer_FindRunnerByID(t *testing.T) {
	testCases := []struct {
		name        string
		runnerID    int64
		mock        func(ctrl *gomock.Controller) *runnermocks.MockService
		wantErrCode codes.Code
		wantName    string
	}{
		{
			name:     "成功查询执行单元",
			runnerID: 5,
			mock: func(ctrl *gomock.Controller) *runnermocks.MockService {
				svc := runnermocks.NewMockService(ctrl)
				svc.EXPECT().FindByID(gomock.Any(), int64(5)).Return(domain.Runner{
					ID: 5, Name: "ansible-runner", ProgramKind: domain.ProgramProject,
				}, nil)
				return svc
			},
			wantName: "ansible-runner",
		},
		{
			name:     "执行单元不存在返回NotFound",
			runnerID: 404,
			mock: func(ctrl *gomock.Controller) *runnermocks.MockService {
				svc := runnermocks.NewMockService(ctrl)
				svc.EXPECT().FindByID(gomock.Any(), int64(404)).Return(domain.Runner{}, errors.New("not found"))
				return svc
			},
			wantErrCode: codes.NotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			svc := tc.mock(ctrl)
			server := NewRunnerServer(svc)

			res, err := server.FindRunnerByID(t.Context(), &runnerv1.FindRunnerByIDRequest{
				Id: tc.runnerID,
			})

			if tc.wantErrCode != codes.OK {
				require.Error(t, err)
				require.Equal(t, tc.wantErrCode, status.Code(err))
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.runnerID, res.GetRunner().GetId())
			require.Equal(t, tc.wantName, res.GetRunner().GetName())
		})
	}
}

func TestRunnerServer_ListRunnersByCodebookID(t *testing.T) {
	ctrl := gomock.NewController(t)
	service := runnermocks.NewMockService(ctrl)
	service.EXPECT().ListByCodebookID(gomock.Any(), int64(12)).Return([]domain.RunnerExecutionSpec{
		{Runner: domain.Runner{
			ID: 3, Name: "linux-runner", CodebookID: 12, ProgramKind: domain.ProgramInline, Tags: []string{"linux"},
		}, Variables: []domain.RunnerVariable{
			{Key: "REGION", Value: "cn"},
			{Key: "TOKEN", Value: "secret", Secret: true},
		}},
	}, nil)
	server := NewRunnerServer(service)

	response, err := server.ListRunnersByCodebookID(t.Context(), &runnerv1.ListRunnersByCodebookIDRequest{
		CodebookId: 12,
	})

	require.NoError(t, err)
	require.Len(t, response.GetRunners(), 1)
	require.Equal(t, int64(3), response.GetRunners()[0].GetId())
	require.Equal(t, "INLINE", response.GetRunners()[0].GetProgramKind())
	require.Equal(t, "cn", response.GetRunners()[0].GetVariables()[0].GetValue())
	require.Equal(t, "[已脱敏]", response.GetRunners()[0].GetVariables()[1].GetValue())
	require.True(t, response.GetRunners()[0].GetVariables()[1].GetSecret())
}
