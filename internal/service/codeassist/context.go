package codeassist

import (
	"context"
	"fmt"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
)

type preparedContext struct {
	project         domain.CodebookProject
	node            domain.Codebook
	base            domain.CodebookVersion
	editorCode      string
	workspaceTree   []domain.WorkspaceNode
	projectWritable bool
}

func (s *service) prepareContext(ctx context.Context, conversation domain.AIConversation,
	request domain.AIChatContext) (preparedContext, error) {
	project, err := s.codebooks.GetProjectByID(ctx, conversation.ProjectID)
	if err != nil {
		return preparedContext{}, err
	}
	tree, err := s.workspace.Tree(ctx, conversation.ProjectID)
	if err != nil {
		return preparedContext{}, err
	}
	prepared := preparedContext{
		project: project, workspaceTree: tree,
		projectWritable: project.Status != domain.CodebookProjectStatusArchived,
	}
	if request.NodeID == 0 {
		return prepared, nil
	}
	if len(request.EditorCode) > maxEditorCodeLength {
		return preparedContext{}, fmt.Errorf("%w: editor context is too large", errs.ErrInvalidParameter)
	}
	node, err := s.codebooks.GetByID(ctx, request.NodeID)
	if err != nil {
		return preparedContext{}, err
	}
	if !node.IsFile() || node.ProjectID != conversation.ProjectID {
		return preparedContext{}, fmt.Errorf("%w: invalid AI Codebook context", errs.ErrInvalidParameter)
	}
	if request.BaseVersionID <= 0 || node.CurrentVersionID != request.BaseVersionID {
		return preparedContext{}, errs.ErrCodebookVersionConflict
	}
	base, err := s.codebooks.GetVersionByID(ctx, request.BaseVersionID)
	if err != nil {
		return preparedContext{}, err
	}
	if base.NodeID != node.ID {
		return preparedContext{}, fmt.Errorf("%w: invalid Codebook base version", errs.ErrInvalidParameter)
	}
	editorCode := request.EditorCode
	if editorCode == "" {
		editorCode = node.Code
	}
	prepared.node = node
	prepared.base = base
	prepared.editorCode = editorCode
	return prepared, nil
}
