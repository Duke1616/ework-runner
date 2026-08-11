package preview

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunReqOnlyRequiresRunnerAndRuntimeInputs(t *testing.T) {
	var req RunReq
	require.NoError(t, json.Unmarshal([]byte(`{
		"runner_id":11,
		"params":{"args":"{}","extra_args":"--syntax-check"}
	}`), &req))
	require.Equal(t, int64(11), req.RunnerID)
	require.Equal(t, map[string]string{"args": "{}", "extra_args": "--syntax-check"}, req.Params)
}
