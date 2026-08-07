package preview

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunReqResolvedProgramPrefersProgramSpec(t *testing.T) {
	program := &ProgramSpec{Kind: "PROJECT", Project: &ProjectProgramSpec{EntryCodebookID: 11}}
	resolved := (RunReq{Program: program, CodebookID: 22, Code: "ignored"}).resolvedProgram()

	require.Same(t, program, resolved)
}

func TestRunReqResolvedProgramAdaptsLegacyCodebookRequest(t *testing.T) {
	resolved := (RunReq{CodebookID: 11, Code: "echo ok"}).resolvedProgram()

	require.Equal(t, &ProgramSpec{Kind: "INLINE", Inline: &InlineProgramSpec{Code: "echo ok"}}, resolved)
}
