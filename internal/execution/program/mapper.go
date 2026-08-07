// Package program 负责将领域程序输入转换为具体执行通道使用的协议对象。
package program

import (
	executorv1 "github.com/Duke1616/etask/api/proto/gen/etask/executor/v1"
	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/sdk/executor"
	executorartifact "github.com/Duke1616/etask/sdk/executor/artifact"
)

// ToProto 将领域程序转换为远程 Executor 使用的 protobuf 消息。
func ToProto(source *domain.Program) (*executorv1.ProgramSource, error) {
	if source == nil {
		return nil, nil
	}
	if err := source.Validate(); err != nil {
		return nil, err
	}
	result := &executorv1.ProgramSource{}
	switch source.Kind {
	case domain.ProgramInline:
		result.Source = &executorv1.ProgramSource_Inline{
			Inline: &executorv1.InlineProgramSource{Code: source.Inline.Code},
		}
	case domain.ProgramProject:
		ref := source.Project.Source
		result.Source = &executorv1.ProgramSource_Project{Project: &executorv1.ProjectProgramSource{
			Source: &executorv1.ProjectSourceRef{
				SourceId: ref.SourceID, ProjectId: ref.ProjectID, SourceRevision: ref.SourceRevision,
				Digest: ref.Digest, BlobChecksum: ref.BlobChecksum, Size: ref.Size,
				Format: ref.Format, FormatVersion: ref.FormatVersion,
			},
			EntryPoint: source.Project.EntryPoint,
		}}
	}
	return result, nil
}

// ToExecutor 将领域程序转换为内嵌 Agent 使用的 SDK 输入及项目源码引用。
func ToExecutor(source *domain.Program) (*executor.Program, *executorartifact.SourceRef, error) {
	if source == nil {
		return nil, nil, nil
	}
	if err := source.Validate(); err != nil {
		return nil, nil, err
	}
	result := &executor.Program{Kind: executor.ProgramKind(source.Kind)}
	switch source.Kind {
	case domain.ProgramInline:
		result.Inline = &executor.InlineProgram{Code: source.Inline.Code}
		return result, nil, nil
	case domain.ProgramProject:
		ref := source.Project.Source
		result.Project = &executor.ProjectProgram{EntryPoint: source.Project.EntryPoint}
		return result, &executorartifact.SourceRef{
			SourceID: ref.SourceID, Digest: ref.Digest, BlobChecksum: ref.BlobChecksum,
			Size: ref.Size, Format: ref.Format, FormatVersion: ref.FormatVersion,
		}, nil
	}
	return result, nil, nil
}
