package execute

import (
	"github.com/Duke1616/ecmdb/internal/execute/internal/domain"
	"github.com/Duke1616/ecmdb/internal/execute/internal/service"
	"github.com/Duke1616/ecmdb/internal/execute/internal/web"
)

type Service = service.Service

type Worker = domain.Worker

type Handler = web.Handler
