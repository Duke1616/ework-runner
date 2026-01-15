package picker

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/Duke1616/ework-runner/internal/domain"
	"github.com/Duke1616/ework-runner/pkg/grpc/registry"
)

// BasePicker 基础随机选择器
type BasePicker struct {
	reg registry.Registry
	rnd *rand.Rand
}

func (b *BasePicker) Name() string {
	return "RandomPicker"
}

// Pick 从可用的执行节点中随机选择一个
func (b *BasePicker) Pick(ctx context.Context, task domain.Task) (nodeID string, err error) {
	// 从 registry 获取所有可用的执行节点
	services, err := b.reg.ListServices(ctx, "builder")
	if err != nil {
		return "", fmt.Errorf("获取执行节点列表失败: %w", err)
	}

	if len(services) == 0 {
		return "", fmt.Errorf("没有可用的执行节点")
	}

	// 随机选择一个节点
	idx := b.rnd.Intn(len(services))
	selectedNode := services[idx]

	return selectedNode.ID, nil
}

func NewBasePicker(reg registry.Registry) ExecutorNodePicker {
	return &BasePicker{
		reg: reg,
		rnd: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}
