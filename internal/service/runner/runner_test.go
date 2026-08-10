package runner_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	repositorymocks "github.com/Duke1616/etask/internal/repository/mocks"
	runnersvc "github.com/Duke1616/etask/internal/service/runner"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gorm.io/gorm"
)

func TestCreateNormalizesExecutionPoolTarget(t *testing.T) {
	testCases := []struct {
		name       string
		runner     domain.Runner
		pool       domain.ExecutionPool
		poolErr    error
		wantTarget string
		wantErr    error
	}{
		{
			name:       "旧 Kafka Topic 归一化为资源池名称",
			runner:     validRunner(domain.RunnerKindKafka, "sd_cdc_docker_net_env_runner"),
			pool:       domain.ExecutionPool{Name: "山东省疾控中心(dockerNet)", Transport: domain.ExecutionTransportMQ},
			wantTarget: "山东省疾控中心(dockerNet)",
		},
		{
			name:       "gRPC 资源池名称保持不变",
			runner:     validRunner(domain.RunnerKindGRPC, "aliyun"),
			pool:       domain.ExecutionPool{Name: "aliyun", Transport: domain.ExecutionTransportGRPC},
			wantTarget: "aliyun",
		},
		{
			name:    "拒绝通道不匹配",
			runner:  validRunner(domain.RunnerKindKafka, "aliyun"),
			pool:    domain.ExecutionPool{Name: "aliyun", Transport: domain.ExecutionTransportGRPC},
			wantErr: errs.ErrInvalidParameter,
		},
		{
			name:    "拒绝不存在的资源池",
			runner:  validRunner(domain.RunnerKindKafka, "missing"),
			poolErr: gorm.ErrRecordNotFound,
			wantErr: errs.ErrInvalidParameter,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			runners := repositorymocks.NewMockRunnerRepository(ctrl)
			pools := repositorymocks.NewMockExecutionPoolRepository(ctrl)
			pools.EXPECT().Find(gomock.Any(), testCase.runner.Target).
				Return(testCase.pool, testCase.poolErr)
			if testCase.wantErr == nil {
				expected := testCase.runner
				expected.Target = testCase.wantTarget
				runners.EXPECT().Create(gomock.Any(), expected).Return(int64(1), nil)
			}
			svc := runnersvc.NewService(runners, pools)

			_, err := svc.Create(context.Background(), testCase.runner)

			if testCase.wantErr != nil {
				require.Error(t, err)
				require.True(t, errors.Is(err, testCase.wantErr))
				return
			}
			require.NoError(t, err)
		})
	}
}

func validRunner(kind domain.RunnerKind, target string) domain.Runner {
	return domain.Runner{
		Name: "runner", CodebookID: 8, ProgramKind: domain.ProgramInline, Kind: kind, Target: target,
		Handler: "shell", Action: domain.RunnerActionRegistered,
	}
}
