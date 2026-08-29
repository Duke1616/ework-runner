# ETask 全局工程规范与 AI 协作准则 (AGENTS.md)

本文档基于 ETask 全局代码模式与架构设计深度梳理，面向所有参与本仓库开发的 AI Agent 与自动化编程助手，定义全仓库通用的架构分层铁律、设计模式、并发安全原则、编码习惯与工具链规范。

---

## 1. 系统核心定位与架构分层

ETask 采用整洁架构（Clean Architecture / 端口与适配器架构），划分为控制面（Scheduler/Manager）、消息总线（Kafka/SSE）、存储持久层与执行面（Agent/Executor/SDK）。

### 1.1 依赖单向流转铁律
$$\text{Transport (Web / gRPC)} \longrightarrow \text{Service} \longrightarrow \text{Repository} \longrightarrow \text{DAO / Storage}$$
- **Domain 层 (`internal/domain/`)**：纯净的业务领域核心，严禁依赖任何 Transport 协议包（Gin/gRPC）或底层持久化框架（GORM/Redis/SQL）。
- **Transport 层 (`internal/web/`, `internal/grpc/`)**：薄适配器层（Thin Adapters）。仅负责 DTO/Proto 校验与转换、从 Context 提取租户/业务凭据（如 `bizid.FromContext`），随后调用 Service 层。**严禁在 Transport 层编写业务逻辑，严禁直接读写 DAO**。
- **Service 层 (`internal/service/`)**：承载全部业务编排、状态机驱动与协调逻辑。禁止反向依赖 Transport 层。
- **Repository 层 (`internal/repository/`)**：定义并实现数据访问抽象，通过专门的转换函数在 DAO 实体与 Domain 领域模型之间进行双向隔离映射。
- **DAO 层 (`internal/repository/dao/`)**：仅关注具体数据库/存储细节（GORM 标签、SQL 构造、事务控制）。
- **SDK 层 (`sdk/executor/`)**：独立的分布式执行端契约与插件运行时，供外部节点或自定义引擎集成，**严禁反向依赖 `internal/` 下的任何私有包**。

---

## 2. 领域建模与设计模式规范

### 2.1 领域对象与状态机
- **强类型枚举**：业务状态、类型统一采用自定义 string 衍生类型定义（如 `TaskStatus`、`TaskExecutionStatus`），并提供 `String()`、`IsValid()`、`ToProto()` 等方法，禁止使用无类型魔法字符串。
- **乐观锁并发控制**：涉及高并发修改的实体（如 Task、TaskExecution、Codebook），必须包含 `Version int64` 字段，并在持久化层实施 CAS 版本核验。
- **扩展属性映射**：DAO 模型中的 JSON 动态字段统一采用 `sqlx.JSONColumn[T]` 泛型封装，杜绝手动反序列化样板代码。

### 2.2 服务与接口命名约定
- **接口与实现命名**：
  - 位于当前业务包内部时，服务接口统一命名为 `Service`（如 `package task; type Service interface`），实现结构体不导出命名为 `service`，构造工厂命名为 `NewService`。
  - 外部消费引入时，采用包别名区分（如 `taskSvc "github.com/Duke1616/etask/internal/service/task"`）。
  - 跨模块核心抽象接口以 `I` 开头（如 `ITaskService`、`IUserRepository`、`IRegistry`）。

---

## 3. 并发安全、容灾与可靠性法则

### 3.1 抢占与防惊群
- **长轮询两阶段调度**：任务抢占调度必须结合数据库行级排他锁 `FOR UPDATE SKIP LOCKED`，防止多调度节点并发抢占引发死锁或惊群效应。

### 3.2 高频并发合并与上下文脱钩
- **并发防雪崩**：高频并发准备任务（如制品下载、依赖挂载）必须使用 `singleflight.Group` 进行同载荷合并。
- **生命周期脱钩**：对于后台必须执行完成的落盘或物化任务，必须使用 `context.WithoutCancel(ctx)` 将底层任务与单个调用方的请求 Context 解耦，避免单个客户端连接超时导致全盘被杀。

### 3.3 任务生命周期与终止控制
- **取消意图优先与终止墓碑**：任务终止支持按 `execution_id` 幂等下发。通过 Outbox 异步广播取消信号，执行端必须在本地维护短期终止墓碑，防止延迟到达的滞留任务被错误启动。
- **终态防覆盖**：晚到的执行上报、重调度或心跳更新必须使用状态 CAS，严禁覆盖 `CANCELLED`、`SUCCESS` 等终态。

### 3.4 文件与磁盘操作的两阶段原子提交
- **临时写入 + 原子生效**：所有解压、制品生成或临时文件物化，必须遵循“在临时目录解压/写入 -> 校验内容完整性 -> 通过不可变标记（如 `.ready`）或原子 `os.Rename` 移入正式目录”的两阶段提交流程，杜绝并发进程读取到半成品数据。
- **资源安全回收**：文件流与临时目录在生命周期结束前必须在 `defer` 中妥善释放，且必须显式检查并处理 `Close()` 错误。

---

## 4. 现代 Go 编码习惯与代码质量

### 4.1 核心语法与工具库习惯
- **切片与时间**：优先使用标准库 `slices`（如 `slices.Clone`、`slices.SortFunc`、`slices.IndexFunc`）；时间对比优先使用 `time.Time.Compare`。
- **集合处理**：集合变换、过滤、去重优先使用 `github.com/samber/lo`（如 `lo.Find`、`lo.Map`、`lo.Filter`、`lo.Uniq`），避免编写冗长冗余的 `for-range` 样板代码。
- **错误包装与哨兵错误**：
  - 核心业务错误统一收敛至 `internal/errs/error.go` 定义。
  - 函数间传递错误必须使用 `fmt.Errorf("...: %w", err)` 附加当前操作上下文并保留根因。

### 4.2 注释与去浮夸风格
- **语言与格式**：所有文档、代码注释、设计说明与交流交互**必须使用简体中文**；代码标识符统一使用清晰英文。
- **注释核心意图**：注释仅用于解释**“为什么这样设计”**（背后的并发安全考量、架构取舍、边界防御机制），严禁写重复代码字面意思的废话。
- **严禁宣传式浮夸词汇**：禁止使用任何夸张宣传性词汇（如“强大无匹”、“完美赋能”等），禁止滥用任何 Emoji 图标。

---

## 5. 构建、依赖注入与测试规范（强制）

### 5.1 自动化构建工具优先
仓库通过 `Taskfile.yaml` 管理自动化流程，修改相应模块后必须执行对应命令：
- **依赖整理**：`go mod tidy`
- **代码生成**：修改 Proto 后执行 `task gen`
- **Mock 生成**：接口调整后执行 `task mocks`（底层触发 `go generate` 生成 typed mocks）
- **依赖注入**：变更/新增组件依赖后执行 `task wire`（代码通过 `ioc/` 下的 Wire 管理，需在 `wire_sets.go` 注册 Provider）
- **数据迁移**：更新数据库结构后执行 `task migrate:up`

### 5.2 单元测试规范（强约束）
- **强制表驱动模式**：所有单元测试必须统一采用 `testCases := []struct { name string, ... }` 切片模式，并使用 `for _, tc := range testCases { t.Run(tc.name, func(t *testing.T) { ... }) }` 运行。
- **Mock 数据隔离**：
  - 依赖跨层接口（Service / Repository / DAO / RPC）的测试统一采用 `mockgen` 生成桩代码。
  - 接口文件头部必须保留标准生成指令：
    `//go:generate go tool mockgen -source=./xxx.go -package=xxxmocks -destination=./mocks/xxx.mock.go -typed`
  - Mock 代码必须统一存放在当前包的 `mocks/` 子目录下，包名为 `<pkg>mocks`。
  - 测试用例内统一使用 `ctrl := gomock.NewController(t)` 管理生命周期，优先使用 `-typed` 强类型安全的 `EXPECT()` 链式断言。
- **竞态检测无告警**：任何核心逻辑修改后，必须运行全套竞态检测并保证通过：
  ```bash
  go test -race ./...
  ```
- **测试环境自包含**：涉及磁盘与文件系统测试必须统一使用 `t.TempDir()`，测试退出时由 Go 框架自动回收，禁止残留任何脏文件或全局状态污染。
