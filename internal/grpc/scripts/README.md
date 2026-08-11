# 脚本运行契约

每次 Shell、Python 或 Ansible 执行都在独立工作区中运行。Shell 和 Python 支持 INLINE、PROJECT 两种来源；Ansible 仅支持 PROJECT。用户通过任务顶层的 `program` 字段选择来源：INLINE 可以直接提供代码或引用单文件 Codebook；PROJECT 指定项目入口 Codebook，运行时拉取完整项目源码并从项目根目录启动。程序来源、参数文件、变量文件和制品挂载均属于本次执行，完成后统一清理。

## 运行时变量

脚本通过以下环境变量读取运行输入。除“始终提供”外，其余变量仅在对应能力存在时注入，脚本使用前应通过 `${VAR:-}` 或 `os.environ.get` 判断。

| 变量 | 提供条件 | 含义 |
| --- | --- | --- |
| `ETASK_WORKSPACE_ROOT` | 始终提供 | 本次执行的独立工作区绝对路径。 |
| `ETASK_PROJECT_ROOT` | PROJECT 来源 | 当前项目的只读挂载根目录，也是默认工作目录。 |
| `ETASK_ARGS_FILE` | Shell / Python | 权限为 `0600` 的 JSON 参数文件；没有参数时内容为 `{}`。 |
| `ETASK_VARIABLES_FILE` | Shell / Python | 权限为 `0600` 的 Runner 变量 JSON 文件；没有变量时内容为 `[]`。 |
| `ETASK_SHELL_ENV_FILE` | 仅 Shell | 权限为 `0600`、可被 Shell 安全 `source` 的变量文件。 |
| `ANSIBLE_HOME` | 仅 Ansible | 位于本次可写工作区中的 Ansible 用户目录。 |
| `ANSIBLE_LOCAL_TEMP` | 仅 Ansible | 位于本次可写工作区中的本地临时目录。 |
| `ETASK_SYSTEM_ROOT` | 存在 SYSTEM 制品 | SYSTEM 制品的只读挂载根目录。 |
| `ETASK_DEPENDENCIES_ROOT` | 存在租户制品 | 当前租户全部具名制品的聚合根目录。 |
| `EWORK_RESULT_FD` | 始终提供 | 结构化结果输出文件描述符，当前固定为 `3`；通常由 `want_result` 封装使用。 |
| `PYTHONPATH` | 存在 Python 制品路径 | 在原进程值前追加 SYSTEM 和租户制品的 Python 导入路径。 |
| `PYTHONUNBUFFERED` | 始终提供 | 固定为 `1`，确保 Python 日志及时输出。 |
| `FORCE_COLOR` | 始终提供 | 固定为 `1`，允许命令输出颜色。 |
| `TERM` | 始终提供 | 固定为 `xterm-256color`。 |

Executor 进程原有的操作系统环境变量也会传给脚本，但它们属于部署环境，不是 etask 稳定契约，不建议业务脚本依赖未显式配置的值。

## Runner 变量

Shell 任务会把 Runner 变量直接注入子进程环境，因此推荐直接读取，不需要再 `source`：

```bash
echo "${KUBECONFIG_PATH:?缺少 KUBECONFIG_PATH}"
curl -H "Authorization: Bearer ${TOKEN:?缺少 TOKEN}" https://example.com
```

确实需要变量文件的 Shell 脚本可以使用：

```bash
source "$ETASK_SHELL_ENV_FILE"
```

Python 任务通过 `ETASK_VARIABLES_FILE` 读取 Runner 变量：

```python
import json
import os

with open(os.environ["ETASK_VARIABLES_FILE"], encoding="utf-8") as file:
    variables = {item["key"]: item["value"] for item in json.load(file)}
```

Shell 中 `ETASK_` 前缀和 `EWORK_RESULT_FD` 是运行时保留名称，Runner 变量不能覆盖它们。密钥变量虽然可以直接读取，但日志系统只会对已声明为密钥的原值做脱敏，脚本仍不应主动输出密钥。

## 参数读取

Shell 示例：

```bash
args=$(<"$ETASK_ARGS_FILE")
```

Python 示例：

```python
import json
import os

with open(os.environ["ETASK_ARGS_FILE"], encoding="utf-8") as file:
    args = json.load(file)
```

旧的 `$1/$2` 和 Python `sys.argv[1]/sys.argv[2]` 输入协议不再支持。

## Ansible 项目

Ansible Handler 仅接收 PROJECT 程序来源，所选 Codebook 文件作为 Playbook 入口。运行时工作目录固定为项目根，因此项目中的 `ansible.cfg`、inventory、roles 和 collections 可以按照标准 Ansible 项目结构组织：

```text
project/
├── ansible.cfg
├── inventory/
├── roles/
│   └── deploy/
└── playbooks/
    └── deploy.yml
```

执行命令等价于：

```bash
cd "$ETASK_PROJECT_ROOT"
ansible-playbook --extra-vars @"$ETASK_WORKSPACE_ROOT/ansible-extra-vars.json" playbooks/deploy.yml
```

`vars` 是用户配置普通剧本变量的统一入口：可以手动维护字符串 KV，也可以引用执行单元变量。`args` 与 Shell、Python 使用相同的统一执行入参协议；Ansible 将完整 JSON 挂载为专用的 `args` Extra Var，工单字段通过 `args.environment` 这类路径读取，并保留数字、布尔值、数组和嵌套对象类型。最终变量写入权限为 `0600` 的 Extra Vars 文件，不会把变量值暴露在进程命令行；SSH 连接凭据由任务默认 `credential_ref` 或 inventory 中的 `etask_credential_ref` 注入。

顶层变量优先级仍为全局变量 < Runner 私有变量；本次执行 `args` 使用独立命名空间，不会隐式覆盖 Runner 顶层变量。若 Runner 已定义同名的顶层 `args` 变量，则本次执行入参优先。连接凭据变量属于系统保留项，不能通过 Runner 变量或 `extra_args` 覆盖。

常用 `ansible-playbook` 选项分别通过 Handler metadata 声明，任务管理页面按 component 渲染：

| 参数 | 命令选项 | 值格式 |
| --- | --- | --- |
| `args` | 挂载为同名 Extra Var | JSON，由工作流等调用方提供 |
| `vars` | `--extra-vars @file` | KV 变量数组或执行单元引用 |
| `inventory` | `--inventory` | 项目内文件的相对路径 |
| `credential_ref` | Agent 本地 SSH 凭据 | inventory 未指定时使用的默认凭据别名 |
| `limit` | `--limit` | 主机或组表达式 |
| `tags` | `--tags` | 逗号分隔字符串 |
| `skip_tags` | `--skip-tags` | 逗号分隔字符串 |
| `check` / `diff` / `become` | 同名开关 | 布尔字符串 |
| `become_user` | `--become-user` | 用户名 |
| `forks` / `verbosity` | 对应并发数和 `-v` 级别 | 整数字符串 |
| `extra_args` | 其他选项 | 命令参数字符串 |

`inventory` 必须是项目内已存在的普通文件；`limit` 用于限制 Playbook 中 `hosts` 的实际执行范围。`tags` 和 `skip_tags` 按逗号拆分。`extra_args` 支持类似 `--start-at-task "Deploy application"` 的写法，并在后端安全拆成独立 argv 后传给 `exec.Cmd`，不会经过 Shell；已经由结构化字段管理的选项不能在其中重复指定。

凭据支持 `private_key` 和 `password` 两种类型。Agent 从 `credential_root` 读取预先登记的认证文件：私钥复制到本次执行工作区，密码只写入权限为 `0600` 的临时 Extra Vars 文件；两者都不会进入命令行、任务参数、执行快照或消息。`known_hosts_file` 为必填项，执行时强制使用 `StrictHostKeyChecking=yes`。私钥和密码文件必须仅所有者可访问，密码认证还要求 Agent 安装 `sshpass`；暂不支持带口令私钥。`ansible_password`、私钥路径和 SSH 安全选项不能通过普通变量或 `extra_args` 覆盖。

```yaml
runtime:
  shell:
    enabled: true
    binary: /bin/bash
  python:
    enabled: true
    binary: python
  ansible:
    enabled: true
    binary: ansible-playbook
    sshpass_binary: sshpass
    credential_root: /run/credentials/etask-ansible
    known_hosts_file: /etc/etask/ssh/known_hosts
    credentials:
      production-linux:
        type: private_key
        username: etask
        private_key_file: production-linux-key
      legacy-linux:
        type: password
        username: root
        password_file: legacy-linux-password
```

`private_key_file` 和 `password_file` 只能填写 `credential_root` 下的文件名，不能填写绝对路径或上级目录。未配置 `type` 时保持向后兼容，按 `private_key` 处理。Agent 启动时会校验目录、认证文件、私钥格式、文件权限、`sshpass` 和 `known_hosts`；配置不安全时节点直接启动失败，而不是带着弱化配置继续运行。

同一次 Playbook 中不同主机需要使用不同凭据时，在静态 YAML 或 INI inventory 中只保存非敏感引用：

```yaml
all:
  children:
    modern_linux:
      vars:
        etask_credential_ref: production-linux
      hosts:
        10.0.1.11: {}
        10.0.1.12: {}
    legacy_linux:
      vars:
        etask_credential_ref: legacy-linux
      hosts:
        10.0.2.11: {}
```

运行时按“主机变量 > 子组变量 > 父组变量 > 任务默认 `credential_ref`”生成逐主机连接计划。同一主机匹配到多个同优先级的不同引用时直接失败；一旦 inventory 启用逐主机凭据，所有 inventory 主机都必须通过分组、主机变量或任务默认值获得凭据。解析只在所选静态 inventory 文件直接包含 `etask_credential_ref` 时启用，不执行 inventory 脚本，也不读取项目中的明文密码。

当前逐主机引用解析范围是所选静态 YAML/INI inventory 文件本身；外部 `group_vars`、`host_vars` 和动态 inventory 中的 `etask_credential_ref` 暂不参与解析。未使用 etask 凭据引用时，原有 Ansible inventory 变量行为保持不变。

Ansible 默认关闭 retry 文件，临时目录和用户目录都位于本次可写工作区；PROJECT 项目挂载仍保持只读。Shell、Python 和 Ansible 只有在 `enabled: true` 时才会注册对应 Handler；关闭后也不会校验该语言的本地命令或专属配置。服务启动时会校验所有已启用语言的命令路径。

## 制品路径

Python 的 SYSTEM 组件固定从 `etask` 命名空间导入。制品存在 `python/` 目录时，该目录直接作为 `etask` 包根；混合语言制品则将整个制品根映射为 `etask`。例如：

```python
from etask.private import util
from etask.third_party.base.want_result import want_result
```

租户制品使用项目配置的英文命名空间，例如制品库 `ops_common`：

```python
from ops_common.private import util
```

运行时不会把 SYSTEM 或租户制品泄漏到无命名空间的顶层 import，避免与任务文件发生隐式冲突。

### 制品引用方向

| 引用方向 | 支持情况 | 写法 |
| --- | --- | --- |
| 当前脚本 → SYSTEM | 支持 | `from etask...` |
| 当前脚本 → 租户制品 | 支持 | `from ops_common...` |
| 租户制品 → SYSTEM | 支持 | `from etask...` |
| 租户制品 A → 租户制品 B | 支持，但应避免循环引用 | `from db_common...` |
| SYSTEM → 租户制品 | 不作为稳定契约支持 | SYSTEM 不能依赖租户环境 |

SYSTEM 包内推荐使用相对导入：

```python
from .want_result import want_result
```

租户制品之间无需单独声明依赖；一次执行会固定当前租户全部已激活制品的发布版本，并排除当前源码项目自身。因为没有显式依赖元数据，工作区只能展示本次注入的制品列表，无法可靠生成逻辑依赖图。跨制品引用应保持单向，避免 A、B 互相 import。

Shell 使用明确的制品根目录：

```bash
source "$ETASK_SYSTEM_ROOT/third_party/utils/want_result.sh"
source "$ETASK_DEPENDENCIES_ROOT/ops_common/scripts/common.sh"
```
