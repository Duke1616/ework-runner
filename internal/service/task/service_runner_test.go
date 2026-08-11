package task

import (
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	runnermocks "github.com/Duke1616/etask/internal/service/runner/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestBindRunnerClearsLegacyParameterBindings(t *testing.T) {
	ctrl := gomock.NewController(t)
	runners := runnermocks.NewMockService(ctrl)
	runners.EXPECT().FindByID(gomock.Any(), int64(18)).Return(domain.Runner{
		ID: 18, CodebookID: 11, ProgramKind: domain.ProgramInline,
		Target: "executor", Handler: "shell", Action: domain.RunnerActionRegistered,
	}, nil)
	service := &service{runnerSvc: runners}
	task := domain.Task{
		RunnerID: 18,
		Metadata: map[string]string{"variables": "runner"},
		GrpcConfig: &domain.GrpcConfig{
			Params: map[string]string{"args": `{"id":1}`},
		},
	}

	err := service.bindRunner(t.Context(), &task)

	require.NoError(t, err)
	require.Nil(t, task.Metadata)
	require.Equal(t, "executor", task.GrpcConfig.ServiceName)
	require.Equal(t, "shell", task.GrpcConfig.HandlerName)
	require.Equal(t, `{"id":1}`, task.GrpcConfig.Params["args"])
}
