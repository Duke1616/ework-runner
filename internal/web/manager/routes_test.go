package manager

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPrivateRoutesUseStableStreamPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(nil, nil, nil, nil, nil).PrivateRoutes(router)

	routes := make(map[string]string)
	for _, route := range router.Routes() {
		routes[route.Path] = route.Method
	}

	expected := []string{
		"/api/streams/manager/task-events",
		"/api/streams/manager/tasks/:id/executions",
		"/api/streams/manager/executions/:id/logs",
	}
	for _, path := range expected {
		require.Equal(t, "GET", routes[path], path)
	}

	legacy := []string{
		"/api/manager/task-events/stream",
		"/api/manager/tasks/:id/executions/stream",
		"/api/manager/executions/:id/logs/stream",
	}
	for _, path := range legacy {
		_, exists := routes[path]
		require.False(t, exists, path)
	}
}
