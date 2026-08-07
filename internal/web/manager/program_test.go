package manager

import (
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestProgramConversion(t *testing.T) {
	testCases := []struct {
		name string
		vo   *ProgramSpec
		want *domain.ProgramSpec
	}{
		{
			name: "INLINE code",
			vo:   &ProgramSpec{Kind: "INLINE", Inline: &InlineProgramSpec{Code: "echo ok"}},
			want: &domain.ProgramSpec{Kind: domain.ProgramInline,
				Inline: &domain.InlineProgramSpec{Code: "echo ok"}},
		},
		{
			name: "INLINE codebook",
			vo:   &ProgramSpec{Kind: "INLINE", Inline: &InlineProgramSpec{CodebookID: 11}},
			want: &domain.ProgramSpec{Kind: domain.ProgramInline,
				Inline: &domain.InlineProgramSpec{CodebookID: 11}},
		},
		{
			name: "PROJECT",
			vo:   &ProgramSpec{Kind: "PROJECT", Project: &ProjectProgramSpec{EntryCodebookID: 12}},
			want: &domain.ProgramSpec{Kind: domain.ProgramProject,
				Project: &domain.ProjectProgramSpec{EntryCodebookID: 12}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := toDomainProgram(tc.vo)
			require.Equal(t, tc.want, actual)
			require.Equal(t, tc.vo, toProgramVO(actual))
		})
	}
}
