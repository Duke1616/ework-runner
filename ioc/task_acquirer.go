package ioc

import (
	"github.com/Duke1616/ework-runner/internal/repository"
	"github.com/Duke1616/ework-runner/internal/service/acquirer"
)

func InitMySQLTaskAcquirer(taskRepo repository.TaskRepository) acquirer.TaskAcquirer {
	return acquirer.NewTaskAcquirer(taskRepo)
}
