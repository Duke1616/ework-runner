# 脚本运行契约

每次 Shell 或 Python 执行都在独立工作区中运行。两种语言均支持 INLINE 和 PROJECT 来源，用户通过任务顶层的 `program` 字段选择。INLINE 可以直接提供代码或引用单文件 Codebook；PROJECT 指定项目入口 Codebook，运行时拉取项目源码并从项目根目录启动。程序来源、参数文件、变量文件和制品挂载均属于本次执行，完成后统一清理。

## 运行时变量

脚本通过以下环境变量读取运行输入。除“始终提供”外，其余变量仅在对应能力存在时注入，脚本使用前应通过 `${VAR:-}` 或 `os.environ.get` 判断。

| 变量 | 提供条件 | 含义 |
| --- | --- | --- |
| `ETASK_WORKSPACE_ROOT` | 始终提供 | 本次执行的独立工作区绝对路径。 |
| `ETASK_PROJECT_ROOT` | PROJECT 来源 | 当前项目的只读挂载根目录，也是默认工作目录。 |
| `ETASK_ARGS_FILE` | 始终提供 | 权限为 `0600` 的 JSON 参数文件；没有参数时内容为 `{}`。 |
| `ETASK_VARIABLES_FILE` | 始终提供 | 权限为 `0600` 的 Runner 变量 JSON 文件；没有变量时内容为 `[]`。 |
| `ETASK_SHELL_ENV_FILE` | 仅 Shell | 权限为 `0600`、可被 Shell 安全 `source` 的变量文件。 |
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
