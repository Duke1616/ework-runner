package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/Duke1616/etask/internal/repository/dao"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestExecutionPoolFindSupportsLegacyMQTopic(t *testing.T) {
	testCases := []struct {
		name           string
		namePool       dao.ExecutionPool
		nameErr        error
		topicPool      dao.ExecutionPool
		topicErr       error
		wantName       string
		wantTopicCalls int
		wantErr        error
	}{
		{name: "资源池名称优先", namePool: dao.ExecutionPool{Name: "山东省疾控中心(dockerNet)"},
			wantName: "山东省疾控中心(dockerNet)"},
		{name: "旧 Topic 回退到 MQ 资源池", nameErr: gorm.ErrRecordNotFound,
			topicPool: dao.ExecutionPool{Name: "山东省疾控中心(dockerNet)"},
			wantName:  "山东省疾控中心(dockerNet)", wantTopicCalls: 1},
		{name: "名称查询数据库错误不回退", nameErr: errors.New("database unavailable"),
			wantErr: errors.New("database unavailable")},
		{name: "名称和 Topic 都不存在", nameErr: gorm.ErrRecordNotFound,
			topicErr: gorm.ErrRecordNotFound, wantTopicCalls: 1, wantErr: gorm.ErrRecordNotFound},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			daoStub := &executionPoolDAOStub{
				namePool: testCase.namePool, nameErr: testCase.nameErr,
				topicPool: testCase.topicPool, topicErr: testCase.topicErr,
			}
			repo := NewExecutionPoolRepository(daoStub)

			pool, err := repo.Find(context.Background(), "sd_cdc_docker_net_env_runner")

			if testCase.wantErr != nil {
				require.Error(t, err)
				require.Equal(t, testCase.wantErr.Error(), err.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, testCase.wantName, pool.Name)
			}
			require.Equal(t, testCase.wantTopicCalls, daoStub.topicCalls)
		})
	}
}

type executionPoolDAOStub struct {
	dao.ExecutionPoolDAO
	namePool   dao.ExecutionPool
	nameErr    error
	topicPool  dao.ExecutionPool
	topicErr   error
	topicCalls int
}

func (s *executionPoolDAOStub) FindByName(context.Context, string) (dao.ExecutionPool, error) {
	return s.namePool, s.nameErr
}

func (s *executionPoolDAOStub) FindMQByTopic(context.Context, string) (dao.ExecutionPool, error) {
	s.topicCalls++
	return s.topicPool, s.topicErr
}
