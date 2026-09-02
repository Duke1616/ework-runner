# ETASK 全局权限大盘与元数据字典

> 本文档由 `permgen` 基于全仓 AST 静态分析自动生成。请勿手动修改。
>
> 💡 **联动包含机制**：当为角色分配某项操作权限时，系统将**自动附带拥有**其“联动包含”中的权限，无需管理员手动重复勾选（例如：勾选“修改用户”会自动附带拥有“用户详情”权限）。

- **受控业务模块数**: 9
- **受控权限点总数**: 70


## 模块: 脚本引擎/制品仓库 (`artifact`)

- **所属服务**: `task`
- **定义源码**: `internal/web/artifact/handler.go`

| 操作名称 | 完整权限码 | 作用域 | 归属类型 | 暴露状态 | 联动包含权限 | 宿主源码位置 |
|:---|:---|:---|:---|:---|:---|:---|
| 切换制品 | `task:artifact:activate` | 租户级 | 本级 | 正常 | - | `internal/web/artifact/handler.go` 行 43 |
| 发布制品 | `task:artifact:publish` | 租户级 | 本级 | 正常 | - | `internal/web/artifact/handler.go` 行 36 |
| 制品状态 | `task:artifact:status` | 租户级 | 本级 | 静默 (不暴露) | - | `internal/web/artifact/handler.go` 行 46 |
| 制品发布记录 | `task:artifact:view` | 租户级 | 本级 | 正常 | 制品状态 · `task:artifact:status` | `internal/web/artifact/handler.go` 行 39 |

---


## 模块: 脚本引擎/AI 助手 (`code_assist`)

- **所属服务**: `task`
- **定义源码**: `internal/web/codeassist/handler.go`

| 操作名称 | 完整权限码 | 作用域 | 归属类型 | 暴露状态 | 联动包含权限 | 宿主源码位置 |
|:---|:---|:---|:---|:---|:---|:---|
| 创建 AI 会话 | `task:code_assist:add_conversation` | 租户级 | 本级 | 正常 | - | `internal/web/codeassist/handler.go` 行 40 |
| 应用 AI 项目变更 | `task:code_assist:apply_change_set` | 租户级 | 本级 | 正常 | 创建模板 · `task:codebook:add`<br>创建版本 · `task:codebook:add_version`<br>使用版本 · `task:codebook:use_version` | `internal/web/codeassist/handler.go` 行 50 |
| 发送 AI 消息 | `task:code_assist:chat` | 租户级 | 本级 | 正常 | - | `internal/web/codeassist/handler.go` 行 48 |
| AI 会话详情 | `task:code_assist:get_conversation` | 租户级 | 本级 | 静默 (不暴露) | - | `internal/web/codeassist/handler.go` 行 45 |
| AI 会话列表 | `task:code_assist:view` | 租户级 | 本级 | 正常 | AI 会话详情 · `task:code_assist:get_conversation` | `internal/web/codeassist/handler.go` 行 42 |

---


## 模块: 脚本引擎/脚本模板 (`codebook`)

- **所属服务**: `task`
- **定义源码**: `internal/web/codebook/handler.go`

| 操作名称 | 完整权限码 | 作用域 | 归属类型 | 暴露状态 | 联动包含权限 | 宿主源码位置 |
|:---|:---|:---|:---|:---|:---|:---|
| 创建模板 | `task:codebook:add` | 租户级 | 本级 | 正常 | 导入项目文件 · `task:codebook:import` | `internal/web/codebook/handler.go` 行 43 |
| 创建项目 | `task:codebook:add_project` | 租户级 | 本级 | 正常 | - | `internal/web/codebook/handler.go` 行 104 |
| 创建版本 | `task:codebook:add_version` | 租户级 | 本级 | 正常 | - | `internal/web/codebook/handler.go` 行 85 |
| 代码资源子节点 | `task:codebook:children` | 租户级 | 本级 | 静默 (不暴露) | - | `internal/web/codebook/handler.go` 行 47 |
| 删除脚本模板 | `task:codebook:delete` | 租户级 | 本级 | 正常 | - | `internal/web/codebook/handler.go` 行 80 |
| 归档项目 | `task:codebook:delete_project` | 租户级 | 本级 | 正常 | - | `internal/web/codebook/handler.go` 行 127 |
| 下载项目文件 | `task:codebook:download` | 租户级 | 本级 | 静默 (不暴露) | - | `internal/web/codebook/handler.go` 行 76 |
| 更新模板 | `task:codebook:edit` | 租户级 | 本级 | 正常 | - | `internal/web/codebook/handler.go` 行 62 |
| 更新项目 | `task:codebook:edit_project` | 租户级 | 本级 | 正常 | - | `internal/web/codebook/handler.go` 行 123 |
| 模板详情 | `task:codebook:get` | 租户级 | 本级 | 正常 | 下载项目文件 · `task:codebook:download` | `internal/web/codebook/handler.go` 行 58 |
| 项目详情 | `task:codebook:get_project` | 租户级 | 本级 | 静默 (不暴露) | - | `internal/web/codebook/handler.go` 行 113 |
| 版本详情 | `task:codebook:get_version` | 租户级 | 本级 | 正常 | - | `internal/web/codebook/handler.go` 行 93 |
| 导入项目文件 | `task:codebook:import` | 租户级 | 本级 | 静默 (不暴露) | - | `internal/web/codebook/handler.go` 行 72 |
| 项目删除影响 | `task:codebook:project_delete_impact` | 租户级 | 本级 | 静默 (不暴露) | - | `internal/web/codebook/handler.go` 行 135 |
| 删除项目 | `task:codebook:purge_project` | 租户级 | 本级 | 正常 | 项目删除影响 · `task:codebook:project_delete_impact` | `internal/web/codebook/handler.go` 行 140 |
| 可引用项目列表 | `task:codebook:reference_projects` | 租户级 | 本级 | 静默 (不暴露) | - | `internal/web/codebook/handler.go` 行 118 |
| 重命名模板 | `task:codebook:rename` | 租户级 | 本级 | 正常 | 更新模板 · `task:codebook:edit` | `internal/web/codebook/handler.go` 行 65 |
| 恢复项目 | `task:codebook:restore_project` | 租户级 | 本级 | 正常 | - | `internal/web/codebook/handler.go` 行 131 |
| 模板排序 | `task:codebook:sort` | 租户级 | 本级 | 正常 | - | `internal/web/codebook/handler.go` 行 69 |
| 使用版本 | `task:codebook:use_version` | 租户级 | 本级 | 正常 | - | `internal/web/codebook/handler.go` 行 97 |
| 项目列表 | `task:codebook:view_project` | 租户级 | 本级 | 正常 | 项目详情 · `task:codebook:get_project`<br>可引用项目列表 · `task:codebook:reference_projects` | `internal/web/codebook/handler.go` 行 108 |
| 当前绑定执行单元 | `task:codebook:view_runners` | 租户级 | 跨域 (runner) | 静默 (不暴露) | - | `internal/web/runner/handler.go` 行 62 |
| 代码资源树 | `task:codebook:view_tree` | 租户级 | 本级 | 正常 | 代码资源子节点 · `task:codebook:children` | `internal/web/codebook/handler.go` 行 51 |
| 版本列表 | `task:codebook:view_version` | 租户级 | 本级 | 正常 | - | `internal/web/codebook/handler.go` 行 89 |
| 读取制品文件 | `task:codebook:view_workspace_tree` | 租户级 | 本级 | 正常 | - | `internal/web/codebook/handler.go` 行 55 |

---


## 模块: 资源池管理 (`execution-pool`)

- **所属服务**: `task`
- **定义源码**: `internal/web/pool/admin.go`

| 操作名称 | 完整权限码 | 作用域 | 归属类型 | 暴露状态 | 联动包含权限 | 宿主源码位置 |
|:---|:---|:---|:---|:---|:---|:---|
| 管理绑定资源池 | `task:execution-pool:admin_bind` | 系统级 | 本级 | 正常 | - | `internal/web/pool/admin.go` 行 53 |
| 资源池绑定管理列表 | `task:execution-pool:admin_bindings_view` | 系统级 | 本级 | 正常 | `iam:tenant:view_by_ids` | `internal/web/pool/admin.go` 行 49 |
| 管理禁用资源池绑定 | `task:execution-pool:admin_disable` | 系统级 | 本级 | 正常 | - | `internal/web/pool/admin.go` 行 62 |
| 管理启用资源池绑定 | `task:execution-pool:admin_enable` | 系统级 | 本级 | 正常 | - | `internal/web/pool/admin.go` 行 59 |
| 管理解绑资源池 | `task:execution-pool:admin_unbind` | 系统级 | 本级 | 正常 | - | `internal/web/pool/admin.go` 行 56 |
| 资源池管理列表 | `task:execution-pool:admin_view` | 系统级 | 本级 | 正常 | - | `internal/web/pool/admin.go` 行 46 |

---


## 模块: 任务管理 (`manager`)

- **所属服务**: `task`
- **定义源码**: `internal/web/manager/handler.go`

| 操作名称 | 完整权限码 | 作用域 | 归属类型 | 暴露状态 | 联动包含权限 | 宿主源码位置 |
|:---|:---|:---|:---|:---|:---|:---|
| 创建任务 | `task:manager:add` | 租户级 | 本级 | 正常 | - | `internal/web/manager/handler.go` 行 74 |
| 删除任务 | `task:manager:delete` | 租户级 | 本级 | 正常 | 任务详情 · `task:manager:get` | `internal/web/manager/handler.go` 行 88 |
| 更新任务 | `task:manager:edit` | 租户级 | 本级 | 正常 | 任务详情 · `task:manager:get` | `internal/web/manager/handler.go` 行 77 |
| 订阅任务执行事件 | `task:manager:execution_events` | 租户级 | 本级 | 静默 (不暴露) | - | `internal/web/manager/handler.go` 行 64 |
| 查看执行日志流 | `task:manager:execution_logs` | 租户级 | 本级 | 静默 (不暴露) | - | `internal/web/manager/handler.go` 行 68 |
| 执行参数 | `task:manager:execution_parameters` | 租户级 | 本级 | 正常 | 执行记录 · `task:manager:executions` | `internal/web/manager/handler.go` 行 103 |
| 执行记录 | `task:manager:executions` | 租户级 | 本级 | 正常 | 任务列表 · `task:manager:view`<br>订阅任务执行事件 · `task:manager:execution_events` | `internal/web/manager/handler.go` 行 99 |
| 任务详情 | `task:manager:get` | 租户级 | 本级 | 正常 | - | `internal/web/manager/handler.go` 行 85 |
| 任务日志 | `task:manager:logs` | 租户级 | 本级 | 正常 | 执行记录 · `task:manager:executions`<br>查看执行日志流 · `task:manager:execution_logs` | `internal/web/manager/handler.go` 行 94 |
| 运行任务 | `task:manager:start` | 租户级 | 本级 | 正常 | 任务详情 · `task:manager:get` | `internal/web/manager/handler.go` 行 117 |
| 停止任务 | `task:manager:stop` | 租户级 | 本级 | 正常 | 任务详情 · `task:manager:get` | `internal/web/manager/handler.go` 行 113 |
| 订阅任务状态事件 | `task:manager:task_events` | 租户级 | 本级 | 静默 (不暴露) | - | `internal/web/manager/handler.go` 行 60 |
| 终止执行 | `task:manager:terminate` | 租户级 | 本级 | 正常 | 执行记录 · `task:manager:executions` | `internal/web/manager/handler.go` 行 107 |
| 任务列表 | `task:manager:view` | 租户级 | 本级 | 正常 | 订阅任务状态事件 · `task:manager:task_events` | `internal/web/manager/handler.go` 行 81 |

---


## 模块: 脚本引擎/试运行 (`preview`)

- **所属服务**: `task`
- **定义源码**: `internal/web/preview/handler.go`

| 操作名称 | 完整权限码 | 作用域 | 归属类型 | 暴露状态 | 联动包含权限 | 宿主源码位置 |
|:---|:---|:---|:---|:---|:---|:---|
| 执行脚本试运行 | `task:preview:run` | 租户级 | 本级 | 正常 | - | `internal/web/preview/handler.go` 行 37 |
| 查看试运行 | `task:preview:view` | 租户级 | 本级 | 正常 | 查看试运行日志 · `task:preview:view_logs` | `internal/web/preview/handler.go` 行 40 |
| 查看试运行日志 | `task:preview:view_logs` | 租户级 | 本级 | 静默 (不暴露) | - | `internal/web/preview/handler.go` 行 44 |

---


## 模块: 执行资源 (`resource`)

- **所属服务**: `task`
- **定义源码**: `internal/web/resource/handler.go`

| 操作名称 | 完整权限码 | 作用域 | 归属类型 | 暴露状态 | 联动包含权限 | 宿主源码位置 |
|:---|:---|:---|:---|:---|:---|:---|
| 执行资源列表 | `task:resource:view` | 租户级 | 本级 | 正常 | - | `internal/web/resource/handler.go` 行 37 |

---


## 模块: 执行单元 (`runner`)

- **所属服务**: `task`
- **定义源码**: `internal/web/runner/handler.go`

| 操作名称 | 完整权限码 | 作用域 | 归属类型 | 暴露状态 | 联动包含权限 | 宿主源码位置 |
|:---|:---|:---|:---|:---|:---|:---|
| 注册执行单元 | `task:runner:add` | 租户级 | 本级 | 正常 | - | `internal/web/runner/handler.go` 行 41 |
| 删除执行单元 | `task:runner:delete` | 租户级 | 本级 | 正常 | - | `internal/web/runner/handler.go` 行 55 |
| 更新执行单元 | `task:runner:edit` | 租户级 | 本级 | 正常 | 执行单元详情 · `task:runner:get` | `internal/web/runner/handler.go` 行 51 |
| 执行单元详情 | `task:runner:get` | 租户级 | 本级 | 正常 | - | `internal/web/runner/handler.go` 行 48 |
| 批量查询执行单元 | `task:runner:ids` | 租户级 | 本级 | 静默 (不暴露) | - | `internal/web/runner/handler.go` 行 58 |
| 执行单元列表 | `task:runner:view` | 租户级 | 本级 | 正常 | 批量查询执行单元 · `task:runner:ids`<br>当前绑定执行单元 · `task:codebook:view_runners`<br>复用执行单元 · `task:runner:view_exclude_codebook_id` | `internal/web/runner/handler.go` 行 44 |
| 复用执行单元 | `task:runner:view_exclude_codebook_id` | 租户级 | 本级 | 静默 (不暴露) | - | `internal/web/runner/handler.go` 行 66 |

---


## 模块: 全局变量 (`variable`)

- **所属服务**: `task`
- **定义源码**: `internal/web/variable/handler.go`

| 操作名称 | 完整权限码 | 作用域 | 归属类型 | 暴露状态 | 联动包含权限 | 宿主源码位置 |
|:---|:---|:---|:---|:---|:---|:---|
| 创建全局变量 | `task:variable:add` | 租户级 | 本级 | 正常 | - | `internal/web/variable/handler.go` 行 41 |
| 删除全局变量 | `task:variable:delete` | 租户级 | 本级 | 正常 | 全局变量详情 · `task:variable:get` | `internal/web/variable/handler.go` 行 55 |
| 更新全局变量 | `task:variable:edit` | 租户级 | 本级 | 正常 | 全局变量详情 · `task:variable:get` | `internal/web/variable/handler.go` 行 51 |
| 全局变量详情 | `task:variable:get` | 租户级 | 本级 | 正常 | - | `internal/web/variable/handler.go` 行 48 |
| 全局变量列表 | `task:variable:view` | 租户级 | 本级 | 正常 | 全局变量详情 · `task:variable:get` | `internal/web/variable/handler.go` 行 44 |

---


