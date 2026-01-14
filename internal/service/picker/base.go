package picker

import (
	"context"

	"github.com/Duke1616/ecmdb/internal/domain"
)

type BasePicker struct{}

func (b BasePicker) Name() string {
	//TODO implement me
	panic("implement me")
}

func (b BasePicker) Pick(ctx context.Context, task domain.Task) (nodeID string, err error) {
	//TODO implement me
	panic("implement me")
}

func NewBasePicker() ExecutorNodePicker {
	return &BasePicker{}
}
