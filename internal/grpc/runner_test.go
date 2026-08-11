package grpc

import (
	"context"
	"testing"

	runnerv1 "github.com/Duke1616/etask/api/proto/gen/etask/runner/v1"
	"github.com/Duke1616/etask/internal/domain"
	runnermocks "github.com/Duke1616/etask/internal/service/runner/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestRunnerServerListsRunnersByCodebookID(t *testing.T) {
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

	response, err := server.ListRunnersByCodebookID(context.Background(), &runnerv1.ListRunnersByCodebookIDRequest{
		CodebookId: 12,
	})

	require.NoError(t, err)
	require.Len(t, response.GetRunners(), 1)
	require.Equal(t, int64(3), response.GetRunners()[0].GetId())
	require.Equal(t, "INLINE", response.GetRunners()[0].GetProgramKind())
	require.Equal(t, "cn", response.GetRunners()[0].GetVariables()[0].GetValue())
	require.Empty(t, response.GetRunners()[0].GetVariables()[1].GetValue())
	require.True(t, response.GetRunners()[0].GetVariables()[1].GetSecret())
}
