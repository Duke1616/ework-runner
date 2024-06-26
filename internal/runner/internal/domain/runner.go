package domain

// Runner 任务模版唯一标识 + Tag 信息，我就应该知道我要往哪一个工作节点的Topic发送数据
type Runner struct {
	TaskIdentifier string   // 任务模版唯一标识
	TaskSecret     string   // 任务模版密钥
	WorkName       string   // 工作节点名称、唯一的
	Name           string   // 名称
	Tags           []string // 标签
	Desc           string   // 详细信息
}
