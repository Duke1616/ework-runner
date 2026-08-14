package dao

import (
	"strings"
	"testing"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/stretchr/testify/require"
)

func TestTaskNotificationQueriesUseTenantPlugin(t *testing.T) {
	db, recorder := newTaskOverrideDryRunDB(t)
	notificationDAO := &GORMTaskNotificationRuleDAO{db: db}
	ctx := ctxutil.WithTenantID(t.Context(), 9)

	_, err := notificationDAO.FindByTaskID(ctx, 7)
	require.NoError(t, err)
	require.Contains(t, recorder.statement[strings.Index(recorder.statement, "WHERE"):], "tenant_id")
}
