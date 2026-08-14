package notification

import (
	"context"
	"time"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	templatev1 "github.com/Duke1616/etask/api/proto/gen/ealert/template/v1"
	"github.com/gotomicro/ego/core/elog"
)

// TemplateBootstrapTask 在应用启动后异步同步 ETask 的内置通知模板。
type TemplateBootstrapTask struct {
	syncer TemplateSyncer
}

// NewTemplateBootstrapTask 创建内置通知模板启动任务。
func NewTemplateBootstrapTask(client templatev1.TemplateServiceClient) *TemplateBootstrapTask {
	return &TemplateBootstrapTask{syncer: NewTemplateSyncer(client)}
}

// Start 延迟执行模板同步，避免阻塞应用主服务启动。
func (t *TemplateBootstrapTask) Start(ctx context.Context) {
	go func() {
		timer := time.NewTimer(3 * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		systemCtx := ctxutil.WithTenantID(ctx, ctxutil.SystemTenantID)
		if err := t.syncer.SyncAll(systemCtx); err != nil {
			elog.Error("运行 ETask 内置通知模板同步失败", elog.FieldErr(err))
		}
	}()
}
