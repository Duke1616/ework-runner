package repository

import (
	"context"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository/dao"
	"github.com/Duke1616/etask/pkg/sqlx"
	"github.com/stretchr/testify/require"
)

type taskAssociationTaskDAOStub struct {
	dao.TaskDAO
	task *dao.Task
}

func (s *taskAssociationTaskDAOStub) GetByName(context.Context, string) (*dao.Task, error) {
	return s.task, nil
}

type taskAssociationParamDAOStub struct {
	rules   []dao.TaskParamOverrideRule
	pending dao.TaskRunParamOverride
}

func (s *taskAssociationParamDAOStub) FindRulesByTaskID(context.Context,
	int64) ([]dao.TaskParamOverrideRule, error) {
	return s.rules, nil
}

func (s *taskAssociationParamDAOStub) FindPendingByTaskID(context.Context,
	int64) (dao.TaskRunParamOverride, bool, error) {
	return s.pending, true, nil
}

type taskAssociationNotificationDAOStub struct {
	rules []dao.TaskExecutionNotificationRule
}

func (s *taskAssociationNotificationDAOStub) FindByTaskID(context.Context,
	int64) ([]dao.TaskExecutionNotificationRule, error) {
	return s.rules, nil
}

func (s *taskAssociationNotificationDAOStub) FindByTaskIDs(context.Context,
	[]int64) ([]dao.TaskExecutionNotificationRule, error) {
	return s.rules, nil
}

func TestTaskRepositoryGetByNameBuildsAggregate(t *testing.T) {
	taskDAO := &taskAssociationTaskDAOStub{task: &dao.Task{ID: 7, Name: "task"}}
	paramDAO := &taskAssociationParamDAOStub{
		rules: []dao.TaskParamOverrideRule{{
			ParamKey: "region",
			InputConfig: sqlx.JSONColumn[dao.TaskParamOverrideInputConfig]{
				Valid: true,
				Val: dao.TaskParamOverrideInputConfig{
					AllowedModes: []domain.TaskParamInputMode{domain.TaskParamInputModeManual},
					DefaultMode:  domain.TaskParamInputModeManual,
				},
			},
		}},
		pending: dao.TaskRunParamOverride{
			Overrides: sqlx.JSONColumn[map[string]string]{
				Valid: true, Val: map[string]string{"region": "cn"},
			},
		},
	}
	notificationDAO := &taskAssociationNotificationDAOStub{
		rules: []dao.TaskExecutionNotificationRule{{
			TriggerStatus: "FAILED", TemplateSetID: 42, Enabled: true,
		}},
	}
	repo := NewTaskRepository(taskDAO, paramDAO, notificationDAO, nil)

	task, err := repo.GetByName(t.Context(), "task")

	require.NoError(t, err)
	require.Equal(t, "region", task.ParamOverrideRules[0].ParamKey)
	require.Equal(t, "cn", task.PendingParamOverrides["region"])
	require.Equal(t, int64(42), task.NotificationRules[0].TemplateSetID)
}
