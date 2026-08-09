package web

import (
	"reflect"
	"testing"

	"github.com/Duke1616/eiam/pkg/web/capability"
	"github.com/Duke1616/etask/internal/web/artifact"
	"github.com/Duke1616/etask/internal/web/codeassist"
	"github.com/Duke1616/etask/internal/web/codebook"
	"github.com/Duke1616/etask/internal/web/manager"
	"github.com/Duke1616/etask/internal/web/pool"
	"github.com/Duke1616/etask/internal/web/preview"
	"github.com/Duke1616/etask/internal/web/resource"
	"github.com/Duke1616/etask/internal/web/runner"
	"github.com/Duke1616/etask/internal/web/variable"
	"github.com/gin-gonic/gin"
)

type capabilityHandler interface {
	PrivateRoutes(*gin.Engine)
	ProvidePermissions() []capability.Permission
}

func TestPrivateRouteCapabilityDeclarations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	handlers := []capabilityHandler{
		artifact.NewHandler(nil),
		codeassist.NewHandler(nil),
		codebook.NewHandler(nil, nil, nil, nil),
		manager.NewHandler(nil, nil, nil, nil),
		pool.NewAdminHandler(nil, nil),
		preview.NewHandler(nil),
		resource.NewHandler(nil),
		runner.NewHandler(nil),
		variable.NewHandler(nil),
	}
	for _, handler := range handlers {
		handler.PrivateRoutes(engine)
	}

	syncedCodes := make(map[string]struct{})
	neededCodes := make(map[string]struct{})
	for _, handler := range handlers {
		for _, permission := range handler.ProvidePermissions() {
			if !permission.NoSync {
				syncedCodes[permission.Code] = struct{}{}
			}
			for _, need := range permission.Needs {
				neededCodes[need] = struct{}{}
			}
		}
	}

	routeByCode := make(map[string]string)
	for _, route := range engine.Routes() {
		info, exists := capability.GetResourceInfo(reflect.ValueOf(route.HandlerFunc).Pointer())
		if !exists {
			t.Fatalf("私有路由 %s %s 缺少 capability 声明", route.Method, route.Path)
		}
		if previous, duplicated := routeByCode[info.Code]; duplicated {
			t.Fatalf("capability code %s 被路由 %s 与 %s 重复使用", info.Code, previous, route.Path)
		}
		routeByCode[info.Code] = route.Path
	}

	for code, path := range routeByCode {
		if _, synced := syncedCodes[code]; synced {
			continue
		}
		if _, needed := neededCodes[code]; !needed {
			t.Errorf("NoSync 路由 %s (%s) 未被任何同步权限 Needs", path, code)
		}
	}
}
