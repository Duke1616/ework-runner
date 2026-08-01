package invoker

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/etask/internal/domain"
	executionevent "github.com/Duke1616/etask/internal/event/execution"
	"github.com/Duke1616/etask/pkg/mpx"
	"github.com/ecodeclub/mq-api"
)

func TestMQInvokerRun(t *testing.T) {
	testCases := []struct {
		name    string
		topic   string
		produce error
		wantErr string
	}{
		{name: "发布完整执行快照", topic: "agent-shell"},
		{name: "资源池缺少 Topic", wantErr: "缺少执行 Topic"},
		{name: "消息发布失败", topic: "agent-shell", produce: context.DeadlineExceeded, wantErr: "发布 Agent 执行命令失败"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			producer := &producerStub{err: testCase.produce}
			queue := &mqStub{producer: producer}
			execution := mqExecutionFixture()
			ctx := ctxutil.WithTenantID(context.Background(), execution.TenantID)
			state, err := NewMQInvoker(queue).Run(ctx, execution, testCase.topic)
			if testCase.wantErr == "" && err != nil {
				t.Fatalf("Run() 返回意外错误: %v", err)
			}
			if testCase.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
					t.Fatalf("Run() 错误 = %v, 期望包含 %q", err, testCase.wantErr)
				}
				return
			}
			if state.Status != domain.TaskExecutionStatusRunning || state.ID != execution.ID {
				t.Fatalf("Run() 状态 = %#v", state)
			}
			if queue.topic != testCase.topic {
				t.Fatalf("Producer Topic = %q, 期望 %q", queue.topic, testCase.topic)
			}
			var command executionevent.Command
			if err = json.Unmarshal(producer.message.Value, &command); err != nil {
				t.Fatalf("解析执行命令失败: %v", err)
			}
			if command.DispatchID == "" || command.ExecutionID != execution.ID ||
				command.TenantID != execution.TenantID || command.Handler != "shell" ||
				command.Source != domain.TaskExecutionSourceTask {
				t.Fatalf("执行命令字段不完整: %#v", command)
			}
			if state.ExecutorNodeID != executionevent.DispatchNodeID(testCase.topic, command.DispatchID) {
				t.Fatalf("执行节点派发标记 = %q", state.ExecutorNodeID)
			}
			if len(command.Artifacts) != 1 || command.Artifacts[0].ReleaseID != 40 {
				t.Fatalf("执行命令制品引用不完整: %#v", command.Artifacts)
			}
			if producer.message.Header[mqx.HeaderTenantID] != "20" {
				t.Fatalf("租户 Header = %q", producer.message.Header[mqx.HeaderTenantID])
			}
		})
	}
}

func TestMQInvokerTerminatePublishesControlCommand(t *testing.T) {
	producer := &producerStub{}
	queue := &mqStub{producer: producer}
	execution := mqExecutionFixture()
	execution.Route = domain.ExecutionRoute{Topic: "agent-shell"}
	ctx := ctxutil.WithTenantID(context.Background(), execution.TenantID)

	err := NewMQInvoker(queue).Terminate(ctx, execution, "管理员强制结束")
	if err != nil {
		t.Fatalf("Terminate() 返回意外错误: %v", err)
	}
	if queue.topic != executionevent.ControlTopic("agent-shell") {
		t.Fatalf("控制 Topic = %q", queue.topic)
	}
	var command executionevent.ControlCommand
	if err = json.Unmarshal(producer.message.Value, &command); err != nil {
		t.Fatalf("解析终止命令失败: %v", err)
	}
	if command.ExecutionID != execution.ID || command.Reason != "管理员强制结束" {
		t.Fatalf("终止命令字段不完整: %#v", command)
	}
	if producer.message.Header[mqx.HeaderTenantID] != "20" {
		t.Fatalf("终止命令租户 Header = %q", producer.message.Header[mqx.HeaderTenantID])
	}
	if len(producer.message.Key) == 0 {
		t.Fatal("终止命令缺少 execution 分区键")
	}
}

func TestMQInvokerTerminateRequiresTenantContext(t *testing.T) {
	execution := mqExecutionFixture()
	execution.Route = domain.ExecutionRoute{Topic: "agent-shell"}

	err := NewMQInvoker(&mqStub{producer: &producerStub{}}).
		Terminate(context.Background(), execution, "管理员强制结束")
	if err == nil || !strings.Contains(err.Error(), "缺少租户上下文") {
		t.Fatalf("Terminate() 错误 = %v", err)
	}
}

func mqExecutionFixture() domain.TaskExecution {
	return domain.TaskExecution{
		ID: 10, TenantID: 20, Source: domain.TaskExecutionSourceTask,
		Task: domain.Task{ID: 30, TenantID: 20, Name: "测试任务", GrpcConfig: &domain.GrpcConfig{
			HandlerName: "shell", Params: map[string]string{"code": "echo ok"},
		}},
		Artifacts: []domain.ArtifactRef{{ReleaseID: 40}},
	}
}

type mqStub struct {
	topic    string
	producer mq.Producer
}

func (m *mqStub) CreateTopic(context.Context, string, int) error { return nil }
func (m *mqStub) DeleteTopics(context.Context, ...string) error  { return nil }
func (m *mqStub) Producer(topic string) (mq.Producer, error) {
	m.topic = topic
	return m.producer, nil
}
func (m *mqStub) Consumer(string, string) (mq.Consumer, error) { return nil, nil }
func (m *mqStub) Close() error                                 { return nil }

type producerStub struct {
	message *mq.Message
	err     error
}

func (p *producerStub) Produce(_ context.Context, message *mq.Message) (*mq.ProducerResult, error) {
	p.message = message
	return &mq.ProducerResult{}, p.err
}
func (p *producerStub) ProduceWithPartition(context.Context, *mq.Message, int) (*mq.ProducerResult, error) {
	return nil, nil
}
func (p *producerStub) Close() error { return nil }
