package event

import (
	"context"
	"encoding/json"
	"testing"

	eventmocks "github.com/Duke1616/etask/internal/agent/event/mocks"
	"github.com/Duke1616/etask/internal/domain"
	executionevent "github.com/Duke1616/etask/internal/event/execution"
	"github.com/ecodeclub/mq-api"
	"go.uber.org/mock/gomock"
)

func TestExecutionEventPublisherBuildsKeyedEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	queue := eventmocks.NewMockMQ(ctrl)
	underlying := eventmocks.NewMockProducer(ctrl)
	queue.EXPECT().Producer(executionevent.EventTopic).Return(underlying, nil)
	var message *mq.Message
	underlying.EXPECT().Produce(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, produced *mq.Message) (*mq.ProducerResult, error) {
			message = produced
			return &mq.ProducerResult{}, nil
		})
	publisher, err := NewExecutionEventPublisher(queue)
	if err != nil {
		t.Fatal(err)
	}
	if err = publisher.PublishLogs(context.Background(), executionevent.LogBatch{
		DispatchID: "dispatch-1", Sequence: 1,
		State: domain.ExecutionState{ID: 10}, Logs: []string{"log"},
	}); err != nil {
		t.Fatal(err)
	}
	if string(message.Key) != "dispatch-1" {
		t.Fatalf("Kafka Key = %q, 期望 dispatch-1", message.Key)
	}
	var event executionevent.Event
	if err = json.Unmarshal(message.Value, &event); err != nil {
		t.Fatal(err)
	}
	if event.EventID != "dispatch-1:log:1" || event.Version != executionevent.SchemaVersion {
		t.Fatalf("执行事件不完整: %#v", event)
	}
}
