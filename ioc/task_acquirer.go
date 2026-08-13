package ioc

import (
	"github.com/Duke1616/etask/internal/repository"
	"github.com/Duke1616/etask/internal/service/acquirer"
	"github.com/Duke1616/etask/internal/sse"
)

func InitMySQLTaskAcquirer(taskRepo repository.TaskRepository, events *sse.Hubs) acquirer.TaskAcquirer {
	return acquirer.NewTaskAcquirer(taskRepo, events)
}
