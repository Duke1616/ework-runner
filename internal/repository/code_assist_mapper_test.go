package repository

import (
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/repository/dao"
	"github.com/stretchr/testify/require"
)

func TestAIChangeSetItemsJSONRoundTrip(t *testing.T) {
	source := domain.AIChangeSet{
		ID: 1, ConversationID: 2, MessageID: 3, ProjectID: 4, BaseRevision: 5,
		Summary: "update", Status: domain.AIChangeSetStatusValidated,
		Items: []domain.AIChangeItem{{
			Operation: domain.AIChangeOperationUpdate, Path: "task.py", NodeID: 10,
			BaseVersionID: 20, BaseHash: "hash", Language: "python", Code: "print(1)\n",
		}},
	}
	entity := toAIChangeSetEntity(source)
	value, err := entity.Items.Value()
	require.NoError(t, err)

	loaded := dao.AIChangeSet{
		ID: entity.ID, ConversationID: entity.ConversationID, MessageID: entity.MessageID,
		ProjectID: entity.ProjectID, BaseRevision: entity.BaseRevision, Summary: entity.Summary,
		Status: entity.Status,
	}
	require.NoError(t, loaded.Items.Scan(value))

	require.Equal(t, source, toAIChangeSetDomain(loaded))
}
