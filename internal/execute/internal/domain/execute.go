package domain

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

type Worker struct {
	Name   string
	Desc   string
	Topic  string
	Status Status
}

type ExecuteReceive struct {
	TaskId    int64
	Language  string
	Code      string
	Args      string
	Variables string
}

type Variable struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}
