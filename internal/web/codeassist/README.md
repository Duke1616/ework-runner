# CodeAssist

CodeAssist 是 Codebook 的 AI 对话与候选代码能力，只在 Scheduler/Web 模式运行。Executor 和 Agent 不依赖 AI 配置。

## 配置

```yaml
ai:
  provider: "rawchat"
  endpoint: "https://rawchat.cn/codex"
  model: "gpt-5.6-sol"
  timeout: "180s"
  max_output_tokens: 8192
  max_concurrency: 4
  reasoning_effort: "low"
```

| Provider | 接入方式 | API Key |
| --- | --- | --- |
| `openai` | OpenAI Responses | `OPENAI_API_KEY` |
| `rawchat` | Responses + RawChat 事件适配 | `RAWCHAT_API_KEY` |
| `qwen` | Eino ToolCallingChatModel | `QWEN_API_KEY` |

Qwen 的 `endpoint` 使用 OpenAI-compatible Chat Completions 地址，并可配置 `enable_thinking`。未配置模型时 Scheduler 仍可启动，AI 接口返回不可用错误。

## 接口与能力

| 路径 | 能力编码 | 说明 |
| --- | --- | --- |
| `POST /conversation/create` | `task:code_assist:add_conversation` | 创建项目会话 |
| `POST /conversation/list` | `task:code_assist:view` | 查询当前用户的项目会话 |
| `POST /conversation/detail` | `task:code_assist:get_conversation` | `NoSync`，依赖 `task:code_assist:view` |
| `POST /message/stream` | `task:code_assist:chat` | 发送消息并接收 SSE |
| `POST /change-set/apply` | `task:code_assist:apply_change_set` | 依赖 Codebook 创建、创建版本和使用版本权限 |

接口前缀为 `/api/code-assist`。会话和候选代码只允许当前用户访问。

## 协作模型

所有项目会话统一使用有界工作区 Agent。前端不负责识别“解释、审阅还是修改”意图，也不通过模式按钮决定工具权限；模型根据用户自然语言和对话历史决定直接回答、读取文件或提出变更。

服务端根据真实上下文装配工具：

- 所有项目会话都提供 `read_workspace_files`。
- 仅当项目可写且当前 Profile 允许修改时提供 `propose_changeset`。
- 已归档项目和只审阅 Profile 没有候选变更工具。

Profile 只补充本轮协作规则，不参与 Agent 路由：

| Profile ID | 用途 |
| --- | --- |
| `default` | 根据自然语言自动解释、审阅或提出变更 |
| `review` | 只读审阅，不生成候选变更 |
| `legacy-migration` | 迁移旧 etask 运行协议 |
| `ansible` | 应用 Ansible 项目结构和凭据约束 |

`profile_id` 为空时使用 `default`。单文件和多文件修改统一保存为 ChangeSet，并执行确定性文件检查。

应用 ChangeSet 会校验项目修订和文件基础版本，在一个事务中创建版本并切换涉及文件的当前版本，但不会自动发布制品。

### 工作区 Agent

Eino ReAct 负责模型与工具之间的有界循环。Harness 最多执行 6 轮、总计不超过 4 分钟。模型可以通过 `read_workspace_files` 按需读取当前项目或只读依赖中的文本文件，单次最多读取 12 个文件，整次运行最多读取 30 个文件和 512 KiB 内容。凭据文件、私钥和包含 Ansible 明文密码字段的内容不会提供给模型。模型没有 Shell、执行、发布或直接写入权限。

Eino 只负责 Agent 编排。工作区权限和路径检查、读取预算、敏感内容过滤、ChangeSet 校验与持久化、Codebook 原子事务以及 SSE 生命周期仍由 CodeAssist Harness 管理。

修改请求最终通过 `propose_changeset` 生成一个包含最多 30 个 `CREATE`/`UPDATE` 操作的项目变更集。会话详情的 `change_sets` 字段返回变更集及逐文件诊断。应用时校验项目 `source_revision`、已有文件的基础版本和哈希，并在一个数据库事务中创建目录、文件和新版本；所有当前版本一起切换，项目源码修订号只递增一次。任何冲突都会回滚整组变更。

当前确定性文件校验支持 Python、Shell 和 YAML；Jinja2、INI 和普通文本保留为项目级校验扩展点。Harness 不会自动执行 Ansible Playbook。

## SSE

- `message.started`
- `message.delta`
- `message.progress`
- `message.completed`
- `message.failed`
- `heartbeat`：空闲时每 15 秒发送，前端不展示。

## 数据表

`ai_conversation`、`ai_message`、`ai_change_set` 由 `AutoMigrate` 创建。ChangeSet 的文件项作为 JSON 整体保存，不建立单独文件项表。迁移 `20260809000007_reset_code_assist_schema.sql` 会删除未投产期间的旧 AI 表和试用数据，再由服务按当前 Profile 模型重建，不提供历史兼容。
