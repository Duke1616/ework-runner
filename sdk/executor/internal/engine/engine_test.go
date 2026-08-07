package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Duke1616/etask/sdk/executor/artifact"
	"github.com/Duke1616/etask/sdk/executor/internal/task"
)

func TestEngineExecute(t *testing.T) {
	handlerErr := errors.New("执行失败")
	testCases := []struct {
		name      string
		artifacts []artifact.Ref
		preparer  artifact.Preparer
		handler   task.TaskHandler
		wantValue string
		wantErr   string
	}{
		{
			name: "无制品时直接执行处理器",
			handler: handlerStub{run: func(ctx *task.Context) error {
				ctx.SetResult("status", "ok")
				return nil
			}},
			wantValue: `{"status":"ok"}`,
		},
		{
			name:      "制品目录会注入任务上下文",
			artifacts: []artifact.Ref{{ReleaseID: 1}},
			preparer: &preparerStub{prepared: &preparedStub{
				roots: task.ArtifactRoots{Default: "/system", Named: map[string]string{"ops_common": "/ops"}},
			}},
			handler: handlerStub{run: func(ctx *task.Context) error {
				roots := ctx.ArtifactRoots()
				ctx.SetResult("roots", roots.Default+":"+roots.Named["ops_common"])
				return nil
			}},
			wantValue: `{"roots":"/system:/ops"}`,
		},
		{
			name:      "声明制品但未配置准备器",
			artifacts: []artifact.Ref{{ReleaseID: 1}},
			handler:   handlerStub{},
			wantErr:   "未配置制品准备器",
		},
		{
			name:    "处理器不存在",
			wantErr: "未找到任务处理器",
		},
		{
			name: "处理器失败时保留结构化结果",
			handler: handlerStub{run: func(ctx *task.Context) error {
				ctx.SetResult("partial", true)
				return handlerErr
			}},
			wantValue: `{"partial":true}`,
			wantErr:   handlerErr.Error(),
		},
		{
			name: "处理器 panic 转换为执行错误并保留结果",
			handler: handlerStub{run: func(ctx *task.Context) error {
				ctx.SetResult("partial", "panic")
				panic("unexpected")
			}},
			wantValue: `{"partial":"panic"}`,
			wantErr:   "任务处理器发生 panic: unexpected",
		},
		{
			name: "不可序列化结果转换为执行错误",
			handler: handlerStub{run: func(ctx *task.Context) error {
				ctx.SetResult("invalid", func() {})
				return nil
			}},
			wantErr: "序列化任务结果失败",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			registry := task.NewHandlerRegistry()
			if testCase.handler != nil {
				if err := registry.Register(testCase.handler); err != nil {
					t.Fatalf("注册测试 Handler 失败: %v", err)
				}
			}
			result, err := New(registry, testCase.preparer).Execute(context.Background(), Command{
				Task:      task.TaskInfo{ExecutionID: 1, TaskID: 2, Name: "测试任务", Handler: "test"},
				Artifacts: testCase.artifacts,
			})
			if testCase.wantErr == "" && err != nil {
				t.Fatalf("Execute() 返回意外错误: %v", err)
			}
			if testCase.wantErr != "" && (err == nil || !strings.Contains(err.Error(), testCase.wantErr)) {
				t.Fatalf("Execute() 错误 = %v, 期望包含 %q", err, testCase.wantErr)
			}
			if result.Value != testCase.wantValue {
				t.Fatalf("Execute() 结果 = %q, 期望 %q", result.Value, testCase.wantValue)
			}
		})
	}
}

func TestEnginePreparesCompleteProjectProgram(t *testing.T) {
	registry := task.NewHandlerRegistry()
	requireNoError(t, registry.Register(handlerStub{run: func(ctx *task.Context) error {
		program := ctx.Program()
		if program == nil || program.Project == nil {
			return errors.New("缺少 PROJECT 程序")
		}
		ctx.SetResult("entry", program.Project.Root+"/"+program.Project.EntryPoint)
		return nil
	}}))
	preparer := &preparerStub{prepared: &preparedStub{sourceRoot: "/cache/project"}}
	program := &task.Program{
		Kind:    task.ProgramProject,
		Project: &task.ProjectProgram{EntryPoint: "playbooks/deploy.yml"},
	}
	projectSource := &artifact.SourceRef{SourceID: 9, Digest: strings.Repeat("a", 64),
		BlobChecksum: strings.Repeat("b", 64), Size: 1, Format: "tar.zst", FormatVersion: 1}

	result, err := New(registry, preparer).Execute(t.Context(), Command{
		Task: task.TaskInfo{ExecutionID: 1, Handler: "test"}, Program: program, ProjectSource: projectSource,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Value != `{"entry":"/cache/project/playbooks/deploy.yml"}` {
		t.Fatalf("Execute() result = %s", result.Value)
	}
	if preparer.source != projectSource {
		t.Fatalf("Prepare() source = %+v", preparer.source)
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

type handlerStub struct {
	run func(*task.Context) error
}

func (h handlerStub) Name() string               { return "test" }
func (h handlerStub) Desc() string               { return "测试处理器" }
func (h handlerStub) Metadata() []task.Parameter { return nil }
func (h handlerStub) Run(ctx *task.Context) error {
	if h.run == nil {
		return nil
	}
	return h.run(ctx)
}

type preparerStub struct {
	prepared *preparedStub
	source   *artifact.SourceRef
	refs     []artifact.Ref
}

func (p *preparerStub) Prune() error { return nil }
func (p *preparerStub) Prepare(_ context.Context, _ artifact.Downloader,
	source *artifact.SourceRef, refs []artifact.Ref) (artifact.PreparedArtifacts, error) {
	p.source = source
	p.refs = append([]artifact.Ref(nil), refs...)
	return p.prepared, nil
}

type preparedStub struct {
	sourceRoot string
	roots      task.ArtifactRoots
}

func (p *preparedStub) SourceRoot() string        { return p.sourceRoot }
func (p *preparedStub) Roots() task.ArtifactRoots { return p.roots }
