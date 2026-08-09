package codebook

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Duke1616/etask/internal/domain"
	"github.com/Duke1616/etask/internal/errs"
	codebookSvc "github.com/Duke1616/etask/internal/service/codebook"
	"github.com/ecodeclub/ginx"
	"github.com/gin-gonic/gin"
)

type workspaceServiceStub struct {
	treeProjectID int64
	systemRootID  int64
}

func (s *workspaceServiceStub) Tree(_ context.Context, projectID int64) ([]domain.WorkspaceNode, error) {
	s.treeProjectID = projectID
	return nil, nil
}

func (s *workspaceServiceStub) SystemTree(_ context.Context, rootID int64) ([]domain.WorkspaceNode, error) {
	s.systemRootID = rootID
	return nil, nil
}

func (s *workspaceServiceStub) ReadArtifactFile(_ context.Context, _, _ int64, _, _ string) (string, error) {
	return "", nil
}

var _ codebookSvc.WorkspaceService = (*workspaceServiceStub)(nil)

func newTreeContext(rawQuery string) *ginx.Context {
	response := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(response)
	ginContext.Request = httptest.NewRequest(http.MethodGet, "/api/codebook/tree/7"+rawQuery, nil)
	ginContext.Params = gin.Params{{Key: "project_id", Value: "7"}}
	return &ginx.Context{Context: ginContext}
}

func TestTreeDefaultsToTenantScope(t *testing.T) {
	workspace := &workspaceServiceStub{}
	handler := &Handler{workspace: workspace}

	_, err := handler.Tree(newTreeContext(""))

	if err != nil {
		t.Fatalf("Tree() 返回错误: %v", err)
	}
	if workspace.treeProjectID != 7 || workspace.systemRootID != 0 {
		t.Fatalf("Tree() 调用 = (tenant: %d, system: %d)，期望 (tenant: 7, system: 0)",
			workspace.treeProjectID, workspace.systemRootID)
	}
}

func TestTreeUsesSystemScopeWhenExplicit(t *testing.T) {
	workspace := &workspaceServiceStub{}
	handler := &Handler{workspace: workspace}

	_, err := handler.Tree(newTreeContext("?scope=SYSTEM"))

	if err != nil {
		t.Fatalf("Tree() 返回错误: %v", err)
	}
	if workspace.treeProjectID != 0 || workspace.systemRootID != 7 {
		t.Fatalf("Tree() 调用 = (tenant: %d, system: %d)，期望 (tenant: 0, system: 7)",
			workspace.treeProjectID, workspace.systemRootID)
	}
}

func TestToWorkspaceNodesIncludesTimes(t *testing.T) {
	handler := &Handler{}
	nodes := handler.toWorkspaceNodes([]domain.WorkspaceNode{{
		Key: "project:1", Name: "site.yml", CTime: 100, UTime: 200,
		Children: []domain.WorkspaceNode{{Key: "project:2", Name: "child.yml", CTime: 300, UTime: 400}},
	}})

	if nodes[0].CTime != 100 || nodes[0].UTime != 200 ||
		nodes[0].Children[0].CTime != 300 || nodes[0].Children[0].UTime != 400 {
		t.Fatalf("toWorkspaceNodes() 未完整返回节点时间: %#v", nodes)
	}
}

func TestProjectAuxiliaryRoutesReuseProjectPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHandler(nil, nil, nil, nil)
	handler.PrivateRoutes(gin.New())

	internalCodes := []string{
		"task:codebook:import",
		"task:codebook:download",
		"task:codebook:get_project",
		"task:codebook:reference_projects",
		"task:codebook:project_delete_impact",
	}
	for _, code := range internalCodes {
		permission, exists := handler.GetPermission(code)
		if !exists {
			t.Fatalf("权限 %s 未注册", code)
		}
		if !permission.NoSync || len(permission.Needs) != 0 {
			t.Fatalf("内部权限 %s 的声明不正确: %#v", code, permission)
		}
	}

	assertNeeds(t, handler, "task:codebook:add", []string{"task:codebook:import"})
	assertNeeds(t, handler, "task:codebook:get", []string{"task:codebook:download"})
	assertNeeds(t, handler, "task:codebook:view_project", []string{
		"task:codebook:get_project",
		"task:codebook:reference_projects",
	})
	assertNeeds(t, handler, "task:codebook:purge_project", []string{
		"task:codebook:project_delete_impact",
	})
}

func assertNeeds(t *testing.T, handler *Handler, code string, expected []string) {
	t.Helper()
	permission, exists := handler.GetPermission(code)
	if !exists {
		t.Fatalf("权限 %s 未注册", code)
	}
	if len(permission.Needs) != len(expected) {
		t.Fatalf("权限 %s 的 Needs = %v，期望 %v", code, permission.Needs, expected)
	}
	for i := range expected {
		if permission.Needs[i] != expected[i] {
			t.Fatalf("权限 %s 的 Needs = %v，期望 %v", code, permission.Needs, expected)
		}
	}
}

func TestTranslateError(t *testing.T) {
	testCases := []struct {
		name     string
		err      error
		wantCode int
		wantMsg  string
	}{
		{
			name:     "名称冲突",
			err:      fmt.Errorf("%w：deploy.sh", errs.ErrCodebookNameConflict),
			wantCode: CodebookNameConflictCode,
			wantMsg:  "同级目录下已存在同名文件或目录：deploy.sh",
		},
		{name: "参数非法", err: errs.ErrInvalidParameter, wantCode: InvalidParameterCode, wantMsg: "参数非法"},
		{name: "系统错误", err: fmt.Errorf("database unavailable"), wantCode: SystemErrorCode, wantMsg: "系统错误"},
	}

	handler := &Handler{}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result := handler.translateError(testCase.err)
			if result.Code != testCase.wantCode || result.Msg != testCase.wantMsg {
				t.Fatalf("translateError() = (%d, %q), 期望 (%d, %q)",
					result.Code, result.Msg, testCase.wantCode, testCase.wantMsg)
			}
		})
	}
}
