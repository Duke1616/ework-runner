package program

import (
	"strings"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestProjectProgramConversions(t *testing.T) {
	source := &domain.Program{Kind: domain.ProgramProject, Project: &domain.ProjectProgram{
		Source: domain.ProjectSourceRef{
			SourceID: 1, ProjectID: 7, SourceRevision: 3,
			Digest: strings.Repeat("a", 64), BlobChecksum: strings.Repeat("b", 64),
			Size: 128, Format: "tar.zst", FormatVersion: 1,
		},
		EntryPoint: "playbooks/deploy.yml",
	}}

	protoSource, err := ToProto(source)
	require.NoError(t, err)
	require.Equal(t, "playbooks/deploy.yml", protoSource.GetProject().GetEntryPoint())
	require.Equal(t, int64(1), protoSource.GetProject().GetSource().GetSourceId())

	executorProgram, projectSource, err := ToExecutor(source)
	require.NoError(t, err)
	require.Equal(t, "playbooks/deploy.yml", executorProgram.Project.EntryPoint)
	require.Equal(t, int64(1), projectSource.SourceID)
}
