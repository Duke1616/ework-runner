package runner

import (
	"context"
	"errors"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	"github.com/Duke1616/etask/internal/repository"
	"github.com/stretchr/testify/require"
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
			runners := &runnerRepositoryStub{}
			svc := NewService(runners, &executionPoolRepositoryStub{pool: testCase.pool, err: testCase.poolErr})

			_, err := svc.Create(context.Background(), testCase.runner)

			if testCase.wantErr != nil {
				require.Error(t, err)
				require.True(t, errors.Is(err, testCase.wantErr))
				require.Zero(t, runners.createCalls)
				return
			}
			require.NoError(t, err)
			require.Equal(t, testCase.wantTarget, runners.created.Target)
		})
	}
}

func validRunner(kind domain.RunnerKind, target string) domain.Runner {
	return domain.Runner{
		Name: "runner", CodebookID: 8, Kind: kind, Target: target,
		Handler: "shell", Action: domain.RunnerActionRegistered,
	}
}

type runnerRepositoryStub struct {
	repository.RunnerRepository
	created     domain.Runner
	createCalls int
}

func (s *runnerRepositoryStub) Create(_ context.Context, runner domain.Runner) (int64, error) {
	s.createCalls++
	s.created = runner
	return 1, nil
}

type executionPoolRepositoryStub struct {
	repository.ExecutionPoolRepository
	pool domain.ExecutionPool
	err  error
}

func (s *executionPoolRepositoryStub) Find(context.Context, string) (domain.ExecutionPool, error) {
	return s.pool, s.err
}
