package event

const TaskRunnerEventName = "task_runner_events"

type Action uint8

func (s Action) ToUint8() uint8 {
	return uint8(s)
}

const (
	// REGISTER 注册
	REGISTER Action = 1
	// UNREGISTER 注销
	UNREGISTER Action = 2
)

type TaskRunnerEvent struct {
	CodebookUid    string
	CodebookSecret string
	WorkerName     string
	Name           string
	Tags           []string
	Desc           string
	Action         Action
}
