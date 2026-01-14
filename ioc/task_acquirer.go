package ioc

import (
	"github.com/Duke1616/ecmdb/internal/repository"
	"github.com/Duke1616/ecmdb/internal/service/acquirer"
)

func InitMySQLTaskAcquirer(taskRepo repository.TaskRepository) acquirer.TaskAcquirer {
	return acquirer.NewTaskAcquirer(taskRepo)
}
