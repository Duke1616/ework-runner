package worker

import (
	"github.com/Duke1616/ecmdb/internal/worker/internal/event"
	"github.com/Duke1616/ecmdb/internal/worker/internal/service"
	"github.com/Duke1616/ecmdb/internal/worker/internal/web"
)

type Module struct {
	Hdl   *web.Handler
	Svc   service.Service
	Event event.TaskWorkerEventProducer
}
