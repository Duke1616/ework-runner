package codeassist

import (
	"fmt"
	"strings"

	"github.com/Duke1616/etask/internal/errs"
)

const (
	defaultProfileID   = "default"
	reviewProfileID    = "review"
	migrationProfileID = "legacy-migration"
	ansibleProfileID   = "ansible"
)

// assistantProfile 只描述本轮协作约束。工作区范围和可用工具由服务端上下文决定。
type assistantProfile struct {
	ID            string
	Version       string
	AllowsChanges bool
	Instructions  string
}

var assistantProfiles = map[string]assistantProfile{
	defaultProfileID: {
		ID: defaultProfileID, Version: "1", AllowsChanges: true,
		Instructions: `根据用户的实际请求解释、审阅或修改当前项目。
保持现有项目结构和业务行为，不要顺便重写与目标无关的代码。`,
	},
	reviewProfileID: {
		ID: reviewProfileID, Version: "1",
		Instructions: `本轮只进行审阅和解释。
指出有依据的正确性、安全性、运行契约、依赖和可维护性问题，不生成候选变更。`,
	},
	migrationProfileID: {
		ID: migrationProfileID, Version: "1", AllowsChanges: true,
		Instructions: `将相关脚本迁移到当前 etask 运行契约，并保持业务行为：
- Shell 和 Python 参数分别改为从 ETASK_ARGS_FILE、ETASK_VARIABLES_FILE 读取。
- SYSTEM 依赖改用 etask 命名空间或 ETASK_SYSTEM_ROOT。
- 租户依赖改用具名命名空间或 ETASK_DEPENDENCIES_ROOT。
- 结构化结果改用 EWORK_RESULT_FD 协议。`,
	},
	ansibleProfileID: {
		ID: ansibleProfileID, Version: "1", AllowsChanges: true,
		Instructions: `按照 Ansible 项目规范处理当前请求：
- 优先检查 ansible.cfg、入口 Playbook、inventory 和相关 Role。
- 遵循已有目录结构、FQCN 用法和命名风格。
- 不在项目文件中写入密码、私钥等凭据，只使用 credential_ref 或 etask_credential_ref 引用。`,
	},
}

func resolveProfile(id string) (assistantProfile, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		id = defaultProfileID
	}
	profile, exists := assistantProfiles[id]
	if !exists {
		return assistantProfile{}, fmt.Errorf("%w: unsupported AI profile: %s",
			errs.ErrInvalidParameter, id)
	}
	return profile, nil
}
