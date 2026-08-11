package resource

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseHandlersPreservesProgramKinds(t *testing.T) {
	handlers := parseHandlers(`[{"name":"shell","desc":"Shell","metadata":[{"key":"variables","role":"variables"}],"program_kinds":["INLINE","PROJECT"]}]`)

	require.Len(t, handlers, 1)
	require.Equal(t, []string{"INLINE", "PROJECT"}, handlers[0].ProgramKinds)
	require.Equal(t, "variables", handlers[0].Metadata[0].Role)
}
