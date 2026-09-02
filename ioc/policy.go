package ioc

import (
	"github.com/Duke1616/eiam/pkg/web/capability"
	"github.com/Duke1616/eiam/pkg/web/capability/syncer"
	httpreporter "github.com/Duke1616/eiam/pkg/web/capability/syncer/http"
	"github.com/Duke1616/eiam/pkg/web/sdk"
)

func InitPolicySDK() *sdk.SDK {
	return sdk.NewSDK()
}

// InitPermSyncer 初始化资产同步器。
// httpreporter.New() 自动从 viper 读取 policy.discovery_url 与 policy.discovery_token，
// 若配置了 token 则自动注入 Authorization: Bearer 头，无需额外手动配置。
func InitPermSyncer() syncer.Syncer {
	return syncer.New(httpreporter.New())
}

// InitProviders 注册逻辑权限供应源。
//
// 得益于 capability SDK 的全自动发现机制（globalRegistries），凡是通过
// capability.NewRegistry 实例化的 Handler 均会自动完成资产上报，
// 开发者无需再手动将 Handler 注入到此列表中。
//
// 仅当存在"纯逻辑权限"（如：不直接关联任何 Web 路由的后台 Job 权限）时，
// 才需在此处手动构建并返回自定义的 PermissionProvider。
func InitProviders() []capability.PermissionProvider {
	return nil
}
