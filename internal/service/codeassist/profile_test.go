package codeassist

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveProfile(t *testing.T) {
	testCases := []struct {
		id            string
		wantID        string
		allowsChanges bool
	}{
		{id: "", wantID: defaultProfileID, allowsChanges: true},
		{id: reviewProfileID, wantID: reviewProfileID},
		{id: migrationProfileID, wantID: migrationProfileID, allowsChanges: true},
		{id: ansibleProfileID, wantID: ansibleProfileID, allowsChanges: true},
	}
	for _, testCase := range testCases {
		profile, err := resolveProfile(testCase.id)
		require.NoError(t, err)
		require.Equal(t, testCase.wantID, profile.ID)
		require.Equal(t, testCase.allowsChanges, profile.AllowsChanges)
	}
}

func TestResolveProfileRejectsUnknownProfile(t *testing.T) {
	_, err := resolveProfile("unknown")
	require.ErrorContains(t, err, "unsupported AI profile")
}
