package codeassist

import (
	"context"
	"fmt"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	"github.com/Duke1616/etask/internal/service/codeassist/recipe"
)

type preparedContext struct {
	project       domain.CodebookProject
	node          domain.Codebook
	base          domain.CodebookVersion
	editorCode    string
	workspaceTree []domain.WorkspaceNode
}

func (s *service) prepareContext(ctx context.Context, conversation domain.AIConversation,
	request domain.AIChatContext, selectedRecipe recipe.Definition) (preparedContext, error) {
	if request.NodeID == 0 {
		if selectedRecipe.RequiresFileContext {
			return preparedContext{}, fmt.Errorf("%w: AI recipe requires a Codebook file context",
				errs.ErrInvalidParameter)
		}
		if !selectedRecipe.UsesWorkspaceAgent {
			return preparedContext{}, nil
		}
		project, err := s.codebooks.GetProjectByID(ctx, conversation.ProjectID)
		if err != nil {
			return preparedContext{}, err
		}
		tree, err := s.workspace.Tree(ctx, conversation.ProjectID)
		if err != nil {
			return preparedContext{}, err
		}
		return preparedContext{project: project, workspaceTree: tree}, nil
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
	tree, err := s.workspace.Tree(ctx, conversation.ProjectID)
	if err != nil {
		return preparedContext{}, err
	}
	var project domain.CodebookProject
	if selectedRecipe.AllowsChanges {
		project, err = s.codebooks.GetProjectByID(ctx, conversation.ProjectID)
		if err != nil {
			return preparedContext{}, err
		}
	}
	return preparedContext{
		project: project, node: node, base: base, editorCode: editorCode, workspaceTree: tree,
	}, nil
}
