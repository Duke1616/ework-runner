package execute

import (
	"github.com/Duke1616/ework-runner/internal/execute/internal/event"
	"github.com/Duke1616/ework-runner/internal/execute/internal/service"
	"github.com/Duke1616/ework-runner/internal/execute/internal/web"
)

type Module struct {
	Hdl *web.Handler
	Svc service.Service
	c   *event.ExecuteConsumer
}
