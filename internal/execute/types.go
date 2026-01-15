package execute

import (
	"github.com/Duke1616/ework-runner/internal/execute/internal/domain"
	"github.com/Duke1616/ework-runner/internal/execute/internal/service"
	"github.com/Duke1616/ework-runner/internal/execute/internal/web"
)

type Service = service.Service

type Worker = domain.Worker

type Handler = web.Handler
