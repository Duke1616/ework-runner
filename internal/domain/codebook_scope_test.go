package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodebookScopeValidateWriteAccess(t *testing.T) {
	const systemTenantID int64 = 1

	require.NoError(t, CodebookScopeTenant.ValidateWriteAccess(10, systemTenantID))
	require.NoError(t, CodebookScopeSystem.ValidateWriteAccess(systemTenantID, systemTenantID))
	require.ErrorContains(t,
		CodebookScopeSystem.ValidateWriteAccess(10, systemTenantID),
		"只有系统租户",
	)
	require.ErrorContains(t,
		CodebookScopeTenant.ValidateWriteAccess(0, systemTenantID),
		"缺少租户上下文",
	)
}

func TestCodebookVersionAllowsEmptyFile(t *testing.T) {
	version := CodebookVersionCreate{NodeID: 1}
	err := version.PrepareForNode(Codebook{ID: 1, Kind: CodebookKindFile, Scope: CodebookScopeTenant})
	require.NoError(t, err)
}

func TestNormalizeCodebookName(t *testing.T) {
	name, err := NormalizeCodebookName(" main.py ")
	require.NoError(t, err)
	require.Equal(t, "main.py", name)

	for _, invalid := range []string{"", ".", "..", "dir/main.py", `dir\\main.py`, strings.Repeat("字", 129)} {
		_, err = NormalizeCodebookName(invalid)
		require.Error(t, err, invalid)
	}
}

func TestCodebookVersionCreateValidatesWriteOptions(t *testing.T) {
	node := Codebook{ID: 1, Kind: CodebookKindFile, Scope: CodebookScopeTenant}
	for _, testCase := range []CodebookVersionCreate{
		{NodeID: 1, ExpectedCurrentVersionID: -1},
		{NodeID: 1, SourceKey: strings.Repeat("x", 129)},
	} {
		require.Error(t, testCase.PrepareForNode(node))
	}
}
