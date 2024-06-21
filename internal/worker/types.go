package worker

import (
	"github.com/Duke1616/ecmdb/internal/worker/internal/domain"
	"github.com/Duke1616/ecmdb/internal/worker/internal/event"
	"github.com/Duke1616/ecmdb/internal/worker/internal/service"
	"github.com/Duke1616/ecmdb/internal/worker/internal/web"
)

type Service = service.Service

type Worker = domain.Worker

type Event = event.TaskWorkerEventProducer

type Handler = web.Handler
