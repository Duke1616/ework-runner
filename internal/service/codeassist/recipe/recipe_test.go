package recipe

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCatalogLoadsRecipes(t *testing.T) {
	catalog := NewCatalog()
	require.Len(t, catalog.items, 5)

	testCases := []struct {
		id             string
		requiresFile   bool
		allowsChanges  bool
		workspaceAgent bool
	}{
		{id: GeneralID, allowsChanges: true},
		{id: "codebook.review", requiresFile: true},
		{id: "codebook.edit", requiresFile: true, allowsChanges: true},
		{id: "codebook.legacy-migration", requiresFile: true, allowsChanges: true},
		{id: AnsibleProjectID, allowsChanges: true, workspaceAgent: true},
	}
	for _, testCase := range testCases {
		t.Run(testCase.id, func(t *testing.T) {
			definition, err := catalog.Get(testCase.id)
			require.NoError(t, err)
			require.Equal(t, testCase.requiresFile, definition.RequiresFileContext)
			require.Equal(t, testCase.allowsChanges, definition.AllowsChanges)
			require.Equal(t, testCase.workspaceAgent, definition.UsesWorkspaceAgent)
			require.NotEmpty(t, definition.Instructions)
		})
	}
}

func TestCatalogUsesGeneralRecipeByDefault(t *testing.T) {
	definition, err := NewCatalog().Get("")
	require.NoError(t, err)
	require.Equal(t, GeneralID, definition.ID)
}
