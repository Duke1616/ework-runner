package domain

type Status uint8

func (s Status) ToUint8() uint8 {
	return uint8(s)
}

const (
	// RUNNING 启用
	RUNNING Status = 1
	// STOPPING 停止
	STOPPING Status = 2
)

type Action uint8

func (s Action) ToUint8() uint8 {
	return uint8(s)
}

const (
	// Register 注册
	Register Action = 1
	// UnRegister 注销
	UnRegister Action = 2
)

type Worker struct {
	Name   string
	Desc   string
	Topic  string
	Status Status
}

type Message struct {
	Name     string // 执行名称
	UUID     string // 唯一标识
	Language string // 语言
	Code     string // 代码
}
