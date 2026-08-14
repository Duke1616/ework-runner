package notification

import (
	"context"
	"fmt"

	templatev1 "github.com/Duke1616/etask/api/proto/gen/ealert/template/v1"
	"github.com/gotomicro/ego/core/elog"
	"google.golang.org/protobuf/proto"
)

// TemplateSyncer 负责将 ETask 内置通知模板幂等同步到 EAlert。
type TemplateSyncer interface {
	// SyncAll 创建缺失的内置模板，并发布内容发生变化的新版本。
	SyncAll(ctx context.Context) error
}

type templateSyncer struct {
	client templatev1.TemplateServiceClient
}

// NewTemplateSyncer 创建 ETask 内置通知模板同步器。
func NewTemplateSyncer(client templatev1.TemplateServiceClient) TemplateSyncer {
	return &templateSyncer{client: client}
}

// SyncAll 同步全部内置通知模板；单个模板失败不会阻断其他模板同步。
func (s *templateSyncer) SyncAll(ctx context.Context) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("EAlert 模板服务客户端不能为空")
	}

	elog.Info("ETask 内置通知模板同步开始")
	for _, cfg := range builtinTemplates {
		if err := s.syncTemplate(ctx, cfg); err != nil {
			elog.Warn("同步 ETask 内置通知模板失败", elog.String("name", cfg.name), elog.FieldErr(err))
		}
	}
	elog.Info("ETask 内置通知模板同步完成")
	return nil
}

func (s *templateSyncer) syncTemplate(ctx context.Context, cfg templateConfig) error {
	resolved, err := s.client.ResolveTemplateID(ctx, &templatev1.ResolveTemplateIDRequest{
		BizId:   int64(cfg.business),
		Key:     cfg.key,
		Channel: cfg.channel,
	})
	if err != nil {
		return fmt.Errorf("解析内置模板 ID 失败: %w", err)
	}
	if resolved == nil {
		resolved = &templatev1.ResolveTemplateIDResponse{}
	}

	templateID, err := s.ensureChannelTemplate(ctx, cfg, resolved.GetTemplateId())
	if err != nil {
		return err
	}
	if resolved.GetTemplateSetId() != 0 && resolved.GetTemplateId() == templateID {
		return nil
	}

	return s.ensureTemplateSet(ctx, cfg, templateID)
}

func (s *templateSyncer) ensureChannelTemplate(ctx context.Context, cfg templateConfig, id int64) (int64, error) {
	var (
		tmpl *templatev1.ChannelTemplate
		err  error
	)
	if id > 0 {
		tmpl, err = s.getTemplate(ctx, id)
	} else {
		tmpl, err = s.createAndPublish(ctx, cfg)
	}
	if err != nil {
		return 0, err
	}

	if tmpl.GetScope() != templatev1.Scope_GLOBAL {
		updated := proto.Clone(tmpl).(*templatev1.ChannelTemplate)
		updated.Scope = templatev1.Scope_GLOBAL
		if _, err = s.client.UpdateTemplate(ctx, &templatev1.UpdateTemplateRequest{
			Template: updated,
		}); err != nil {
			return 0, fmt.Errorf("修复内置模板全局作用域失败: %w", err)
		}
	}

	if err = s.upgradeVersionIfNeeded(ctx, tmpl, cfg); err != nil {
		return 0, err
	}
	return tmpl.GetId(), nil
}

func (s *templateSyncer) getTemplate(ctx context.Context, id int64) (*templatev1.ChannelTemplate, error) {
	resp, err := s.client.GetTemplateByID(ctx, &templatev1.GetTemplateByIDRequest{Id: id})
	if err != nil {
		return nil, fmt.Errorf("获取内置模板详情失败: %w", err)
	}
	if resp == nil || resp.GetTemplate() == nil {
		return nil, fmt.Errorf("EAlert 返回空模板详情: id=%d", id)
	}
	return resp.GetTemplate(), nil
}

func (s *templateSyncer) upgradeVersionIfNeeded(ctx context.Context, tmpl *templatev1.ChannelTemplate, cfg templateConfig) error {
	var active *templatev1.ChannelTemplateVersion
	for _, version := range tmpl.GetVersions() {
		if version.GetId() == tmpl.GetActiveVersionId() {
			active = version
			break
		}
	}
	if active != nil && active.GetContent() == cfg.content {
		return nil
	}

	version, err := s.client.CreateTemplateVersion(ctx, &templatev1.CreateTemplateVersionRequest{
		TemplateId: tmpl.GetId(),
		Name:       cfg.versionName,
		Content:    cfg.content,
		Desc:       "ETask 内置模板自愈升级",
	})
	if err != nil {
		return fmt.Errorf("创建内置模板版本失败: %w", err)
	}
	if version == nil || version.GetVersion() == nil {
		return fmt.Errorf("创建内置模板版本返回空数据")
	}
	if _, err = s.client.PublishTemplate(ctx, &templatev1.PublishTemplateRequest{
		TemplateId: tmpl.GetId(),
		VersionId:  version.GetVersion().GetId(),
	}); err != nil {
		return fmt.Errorf("发布内置模板版本失败: %w", err)
	}
	return nil
}

func (s *templateSyncer) createAndPublish(ctx context.Context, cfg templateConfig) (*templatev1.ChannelTemplate, error) {
	resp, err := s.client.CreateTemplate(ctx, &templatev1.CreateTemplateRequest{
		Template: &templatev1.ChannelTemplate{
			Name:        cfg.name,
			Description: cfg.description,
			Channel:     cfg.channel,
			Scope:       templatev1.Scope_GLOBAL,
			Versions: []*templatev1.ChannelTemplateVersion{{
				Name:    cfg.versionName,
				Content: cfg.content,
				Desc:    "ETask 内置模板初始化",
			}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("创建内置模板失败: %w", err)
	}
	if resp == nil || resp.GetTemplate() == nil || len(resp.GetTemplate().GetVersions()) == 0 {
		return nil, fmt.Errorf("创建内置模板返回数据无效")
	}
	tmpl := resp.GetTemplate()
	if _, err = s.client.PublishTemplate(ctx, &templatev1.PublishTemplateRequest{
		TemplateId: tmpl.GetId(),
		VersionId:  tmpl.GetVersions()[0].GetId(),
	}); err != nil {
		return nil, fmt.Errorf("发布内置模板初始版本失败: %w", err)
	}
	return s.getTemplate(ctx, tmpl.GetId())
}

func (s *templateSyncer) ensureTemplateSet(ctx context.Context, cfg templateConfig, templateID int64) error {
	resp, err := s.client.CreateTemplateSet(ctx, &templatev1.CreateTemplateSetRequest{
		Key:         cfg.key,
		BizId:       int64(cfg.business),
		OwnerId:     1,
		Name:        cfg.name,
		Description: cfg.description,
		Scope:       templatev1.Scope_GLOBAL,
		Items: []*templatev1.TemplateSetItem{{
			Channel:    cfg.channel,
			TemplateId: templateID,
		}},
	})
	if err != nil {
		return fmt.Errorf("创建内置模板集失败: %w", err)
	}
	if resp == nil || resp.GetTemplateSet() == nil || resp.GetTemplateSet().GetId() == 0 {
		return fmt.Errorf("创建内置模板集返回数据无效")
	}
	return nil
}
