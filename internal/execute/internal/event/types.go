package event

const ExecuteResultEventName = "result_execute_events"

type Status uint8

func (s Status) ToUint8() uint8 {
	return uint8(s)
}

const (
	// SUCCESS 成功
	SUCCESS Status = 1
	// FAILED 失败
	failed
	FAILED Status = 2
)

type ExecuteResultEvent struct {
	TaskId     int64  `json:"task_id"`
	Result     string `json:"result"`
	WantResult string `json:"want_result"`
	Status     Status `json:"status"`
}

type ExecuteReceive struct {
	TaskId    int64                  `json:"task_id"`
	Language  string                 `json:"language"`
	Code      string                 `json:"code"`
	Args      map[string]interface{} `json:"args"`
	Variables string                 `json:"variables"`
}
