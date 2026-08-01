package execution

import (
	"fmt"
	"strings"
)

const controlTopicSuffix = ".control"

// ControlCommand 是 Scheduler 广播给 Kafka Agent 的执行控制命令。
type ControlCommand struct {
	ExecutionID int64  `json:"execution_id"`
	Reason      string `json:"reason"`
}

func (c ControlCommand) Validate() error {
	if c.ExecutionID <= 0 || strings.TrimSpace(c.Reason) == "" {
		return fmt.Errorf("Agent 终止命令非法: execution_id=%d", c.ExecutionID)
	}
	return nil
}

// ControlTopic 返回执行 Topic 对应的广播控制 Topic。
func ControlTopic(executionTopic string) string {
	return executionTopic + controlTopicSuffix
}
