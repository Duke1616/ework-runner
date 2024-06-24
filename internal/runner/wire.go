//go:build wireinject

package runner

import (
	"github.com/Duke1616/ecmdb/internal/runner/internal/service"
	"github.com/ecodeclub/mq-api"
	"github.com/google/wire"
)

func InitModule(q mq.MQ) (*Module, error) {
	wire.Build(
		service.NewService,
		wire.Struct(new(Module), "*"),
	)
	return new(Module), nil
}
