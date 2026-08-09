package recipe

import (
	"fmt"
	"strings"

	"github.com/Duke1616/etask/internal/errs"
)

const (
	GeneralID        = "codebook.general"
	AnsibleProjectID = "codebook.ansible-project"
)

// Definition 描述一个内置代码助手场景。
type Definition struct {
	ID                  string
	Version             string
	RequiresFileContext bool
	AllowsChanges       bool
	UsesWorkspaceAgent  bool
	Instructions        string
}

// Catalog 保存当前进程可用的代码助手场景。
type Catalog struct {
	items map[string]Definition
}

// NewCatalog 创建固定的代码助手场景目录。
func NewCatalog() *Catalog {
	definitions := builtInDefinitions()
	items := make(map[string]Definition, len(definitions))
	for _, definition := range definitions {
		items[definition.ID] = definition
	}
	return &Catalog{items: items}
}

// Get 返回指定场景；未指定时使用通用场景。
func (c *Catalog) Get(id string) (Definition, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		id = GeneralID
	}
	definition, ok := c.items[id]
	if !ok {
		return Definition{}, fmt.Errorf("%w: unsupported AI recipe: %s", errs.ErrInvalidParameter, id)
	}
	return definition, nil
}

func builtInDefinitions() []Definition {
	return []Definition{
		{
			ID: GeneralID, Version: "1", AllowsChanges: true,
			Instructions: `根据用户的实际请求解释、审阅或修改 Codebook 脚本。
只有用户明确要求生成、修改或修复代码时才提交完整候选文件。
回答应说明结论、关键依据和需要用户确认的风险。`,
		},
		{
			ID: "codebook.review", Version: "1", RequiresFileContext: true,
			Instructions: `只审阅当前文件并给出有依据的问题和建议。
重点检查正确性、安全边界、运行契约、依赖引用和可读性。
不要生成候选代码，也不要声称已经修改文件。`,
		},
		{
			ID: "codebook.edit", Version: "1",
			RequiresFileContext: true, AllowsChanges: true,
			Instructions: `理解用户的修改目标，在保持原业务行为的前提下生成完整候选文件。
解释修改目标、关键变化和需要用户确认的风险。
不要顺便重写与用户目标无关的代码。`,
		},
		{
			ID: "codebook.legacy-migration", Version: "1",
			RequiresFileContext: true, AllowsChanges: true,
			Instructions: `将旧脚本迁移到当前 etask 运行契约，并保持业务行为：
- 将 Shell 的 $1、$2 和 Python 的 sys.argv[1]、sys.argv[2] 替换为文件环境变量。
- 将旧 third_party 引用迁移到 SYSTEM 或 dependencies 的运行时路径。
- 将旧 stdout JSON 返回迁移为 EWORK_RESULT_FD 结果协议。
- 不重写与运行协议无关的业务逻辑。`,
		},
		{
			ID: AnsibleProjectID, Version: "1", AllowsChanges: true, UsesWorkspaceAgent: true,
			Instructions: `处理当前 Ansible 项目时遵循以下工作流：
1. 先根据工作区路径判断需要读取哪些文件，再使用只读工具获取内容。
2. 优先检查 ansible.cfg、入口 playbook、inventory，以及相关 role 的 tasks、defaults、handlers、templates 和 files。
3. 遵循现有项目结构、FQCN 用法和命名风格；不得编造未读取的文件内容。
4. 不在 inventory、变量或模板中写入密码、私钥等凭据；只使用 credential_ref 或 etask_credential_ref 引用。
5. 解释和审阅任务可以直接回答；修改任务通过 propose_changeset 一次提交完整的 CREATE/UPDATE 文件集合。
6. 不调用 Shell、不执行 playbook、不发布制品，也不直接修改项目。`,
		},
	}
}
