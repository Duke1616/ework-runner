package domain

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProgramSpecValidate(t *testing.T) {
	testCases := []struct {
		name      string
		spec      ProgramSpec
		wantError string
	}{
		{name: "INLINE 直接代码", spec: ProgramSpec{Kind: ProgramInline, Inline: &InlineProgramSpec{Code: "echo ok"}}},
		{name: "INLINE Codebook", spec: ProgramSpec{Kind: ProgramInline, Inline: &InlineProgramSpec{CodebookID: 7}}},
		{name: "INLINE 来源互斥", spec: ProgramSpec{Kind: ProgramInline, Inline: &InlineProgramSpec{Code: "echo ok", CodebookID: 7}}, wantError: "只能指定"},
		{name: "PROJECT 入口", spec: ProgramSpec{Kind: ProgramProject, Project: &ProjectProgramSpec{EntryCodebookID: 7}}},
		{name: "PROJECT 缺少入口", spec: ProgramSpec{Kind: ProgramProject, Project: &ProjectProgramSpec{}}, wantError: "入口 Codebook"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.spec.Validate()
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				return
			}
			require.NoError(t, err)
		})
	}
}
