# SupportFlow AI 架构设计

> 状态：M1、M2 已通过验收；账号、工单等后续部分仍为目标设计。
> 更新日期：2026-11-02。
> 业务规则以 [PRD.md](PRD.md) 为准，开发顺序见 [ROADMAP.md](ROADMAP.md)。

## 1. 单体架构与执行流程

```text
HTTP 客户端 / 后续 Vue 演示端
        ↓
Gin 路由与中间件（请求 ID、日志、认证、粗粒度角色检查）
        ↓
Handler（解析输入、参数校验、调用服务、映射 HTTP 响应）
        ↓
Service（数据权限、状态规则、事务边界）
        ↓
Repository（使用 GORM 查询与更新）
        ↓
MySQL

阶段二：Service → AI HTTP 客户端 → 模型 API
阶段三：登录入口 → Redis 限流
```

采用单进程、单数据库的单体应用。不引入微服务、领域事件总线或通用 CRUD 框架。

以客服认领为例：中间件验证身份并加载当前角色 → Handler 解析工单 ID → Service 开启事务 → Repository 锁定工单行 → Service 校验未分配状态 → 更新负责人和状态 → 写入事件 → 提交事务 → 返回结果。Handler 不直接组织复杂业务或数据库事务。

## 2. 技术默认方案

| 项目 | 方案 | 原因 |
| --- | --- | --- |
| Web 框架 | Go + Gin | 延续已有学习路径，练习路由、中间件和参数绑定 |
| 数据库 | MySQL 8.4 LTS，InnoDB，utf8mb4 | 学习关系模型、事务、行锁和索引 |
| ORM | GORM 常规 API | 常见 Go 实践，便于结合 SQL 理解查询 |
| 配置 | 环境变量与 `.env.example` | 依赖少，适合本地和容器运行 |
| 依赖组装 | `main` 手动构造 | 依赖关系可见，不引入依赖注入框架 |
| 日志 | 标准库 `log/slog` | 足够支持结构化日志 |
| 密码 | bcrypt | 使用成熟密码哈希方案，不自制加密算法 |
| JWT | HS256，访问令牌 2 小时 | 单体场景简单，固定校验算法与有效期 |
| 数据库迁移 | 编号 SQL + 项目内 `cmd/migrate` CLI（官方 golang-migrate 引擎） | 空库可重建，变更可审阅，可验证升级路径 |
| 运行 | Docker Compose | 先管理 MySQL，MVP 末加入 API 容器 |
| 测试 | `testing`、`httptest`、真实 MySQL | 区分业务、HTTP 和数据库语义验证 |

初始化时核验并锁定 Go 与依赖的兼容版本，不使用浮动的镜像 `latest` 标签作为可复现交付方式。

计划依赖及用途：Gin 用于 HTTP；GORM 与 MySQL 驱动用于数据库；`github.com/golang-jwt/jwt/v5` 用于 JWT；`golang.org/x/crypto/bcrypt` 用于密码哈希。迁移 CLI 是开发工具，不要求加入应用运行依赖。Redis 客户端直到 B1 才引入；AI 默认使用标准库 `net/http`。

依赖在对应任务说明用途后添加，不在文档交付时安装。

## 3. 目录结构与职责

```text
cmd/
  api/                 # HTTP 服务入口，加载配置与手动组装依赖
  migrate/             # 显式运行 SQL 迁移的独立命令
  admin/               # 管理员、客服初始化命令
internal/
  config/              # 环境变量读取与校验
  handler/             # HTTP 参数与响应映射
  service/             # 权限、状态、事务等业务逻辑
  repository/          # GORM 查询与更新
  model/               # 数据库模型与业务常量
  dto/                 # 请求、响应结构
  middleware/          # 认证、请求 ID、请求日志
  router/              # 路由注册
  database/            # 数据库连接、连接池
  auth/                # JWT 与密码哈希
migrations/            # 编号 SQL up/down 迁移
tests/integration/     # 真实 MySQL 集成测试
docs/
  PRD.md
  ARCHITECTURE.md
  ROADMAP.md
web/                   # A4 才创建 Vue 演示端
```

根目录后续按任务增加 `go.mod`、`go.sum`、README、Dockerfile、Compose 配置和 `.env.example`。结构按需创建，不生成空目录或占位业务代码。

规则：

- Handler 不暴露 GORM 模型，使用 DTO 避免密码哈希或内部字段泄露。
- Service 接收 `context.Context`，不依赖 `gin.Context`。
- Repository 不决定业务权限，但按 Service 要求将归属条件加入查询。
- 接口由消费方按测试需求定义，不给每个结构体机械创建接口。
- 事务内所有 Repository 操作必须使用同一 `*gorm.DB` 事务连接；禁止事务中混用全局连接。
- 不引入 `BaseService`、通用 CRUD、无实际复用需求的 `pkg` 或大型配置框架。

## 4. 数据模型

### 4.1 通用约定

- 主键为自增 `BIGINT UNSIGNED`。
- 数据库、连接会话及应用时间统一 UTC，时间列使用 `DATETIME(6)`；API 输出 RFC 3339。
- 不采用软删除，不提供业务删除接口。
- 字符串枚举使用 `VARCHAR`，迁移建立有限值约束，Service 同步校验。
- 外键采用限制删除，不级联删除业务记录。
- `users`、`tickets` 包含 `created_at/updated_at`；消息、事件为不可变记录，仅有 `created_at`。

### 4.2 核心四表

| 表 | 主要字段 | 约束 |
| --- | --- | --- |
| `users` | `id`、`email VARCHAR(254)`、`password_hash VARCHAR(255)`、`display_name VARCHAR(64)`、`role VARCHAR(16)`、时间字段 | 邮箱唯一，角色仅 `user/agent/admin` |
| `tickets` | `id`、`requester_id`、可空 `assignee_id`、`title VARCHAR(120)`、`description TEXT`、`category VARCHAR(16)`、`priority VARCHAR(16)`、`status VARCHAR(16)`、时间字段 | 创建人必填；关联用户外键；默认分类 `general`、优先级 `normal`、状态 `open` |
| `ticket_messages` | `id`、`ticket_id`、`author_id`、`content TEXT`、`created_at` | 关联工单和作者，不可编辑或删除 |
| `ticket_events` | `id`、`ticket_id`、`actor_id`、`event_type VARCHAR(32)`、`metadata JSON`、`created_at` | 关联工单和操作者；metadata 不保存密码、令牌或重复正文 |

关系为：用户一对多提交工单、客服一对多负责工单、工单一对多消息和事件。

数据库约束基础引用和枚举，Service 保证创建人为普通用户、负责人为客服等跨表业务规则。`open` 必须无负责人，其余状态必须有负责人。

事件类型：`created/message_added/claimed/assigned/reassigned/priority_changed/resolved/reopened/closed`。metadata 保存本事件所需的前后状态、前后负责人、前后优先级或消息 ID，不能变成无约束的请求体备份。

初始索引：

- `users(email)`：唯一约束和登录查询。
- `tickets(requester_id, created_at, id)`：用户列表。
- `tickets(assignee_id, status, created_at, id)`：客服及负责人筛选。
- `tickets(status, assignee_id, created_at, id)`：未分配队列。
- `ticket_messages(ticket_id, id)`、`ticket_events(ticket_id, id)`：有序分页。

索引是初始方案，不宣称覆盖所有筛选组合。第三阶段用实际 SQL 与 `EXPLAIN` 验证是否需要调整。

### 4.3 迁移策略

从 M2 起使用编号迁移。M2 先建立连接和迁移流程，M3 新增用户表，M6 新增工单与事件表，M8 新增消息表，A2 新增 AI 记录表。

迁移显式执行，不在服务启动时运行 `AutoMigrate`。服务的 readiness 检查数据库连接，迁移是否成功由部署步骤单独验证。正常开发通过新增迁移演进，不改写已经应用的迁移文件。down 迁移可能丢数据，只用于明确指定的开发或测试环境，不在运行流程中自动回滚数据库。

M2 实现选择：`go run ./cmd/migrate up|version` 从环境读取数据库配置，避免在命令行拼接含密码的 DSN。该 CLI 只封装官方 golang-migrate 引擎，不自行实现版本管理。基线编号 000001 的 up/down 均为 `SELECT 1`，仅建立迁移版本；不提前建业务表。

GORM 以 MySQL 8.4 为目标创建底层 `database/sql` 连接池，跳过隐式版本查询和自动 Ping，主入口执行初次限时 Ping。配置缺失时拒绝启动；数据库暂不可达只记录警告并继续提供 HTTP 服务，以便 `/healthz` 保持独立，`/readyz` 每次重新 Ping，最长 2 秒。驱动连接、读写超时默认 3 秒；连接池默认最大 10、空闲 5、生命周期 30 分钟。连接使用 UTC 和 utf8mb4，不向响应或日志打印 DSN。API 连接不允许多语句，迁移单独使用允许多语句的连接池。

## 5. 安全与一致性

### 5.1 身份认证

- 邮箱去除首尾空白并统一小写，注册和登录遵循同一规则。
- 密码不 trim；长度限定为 8～72 字节，使用 bcrypt 哈希和比较。不能仅用前端校验或按字符数替代字节校验。
- JWT 密钥从环境变量读取，不使用默认演示密钥；缺失或明显过短时拒绝启动。
- 固定允许 HS256，校验签名、`sub` 和过期时间；`sub` 表示用户 ID。
- 每次认证后从数据库读取用户及当前角色，不相信请求体角色或令牌中的旧角色。
- 登录失败不区分邮箱不存在和密码错误；响应不泄露密码哈希。
- MVP 无刷新令牌、服务端注销或黑名单；客户端退出清除令牌，已签发令牌仍可使用至过期。
- 请求日志不记录 Authorization、密码、工单正文或模型 API Key。

### 5.2 授权与状态

中间件进行认证和粗粒度角色检查，Service 校验资源归属及状态。详情、消息、事件和后续 AI 记录接口必须复用同样的数据可见性规则，防止通过子资源绕过工单权限。

业务列表直接带归属条件，不先查询所有记录再在内存中过滤。管理员读取全部，客服读取自己负责的工单，普通用户读取自己提交的工单；客服队列使用独立摘要 DTO。

### 5.3 事务与并发

认领、分配、转派、优先级调整、回复和状态变化均锁定目标工单行，在同一事务中校验当前权限、状态、更新记录及写事件。创建工单与创建事件同样一起提交。

主要保证：

- 并发认领只成功一次；第二个事务读取已认领状态后返回 `409`。
- 转派和回复串行判权。转派提交前已获得锁的合法回复可以成功；转派提交后的原客服请求被拒绝。
- 回复和解决、关闭按获得工单锁的顺序处理，不产生已关闭后继续新增消息的情况。
- 事件写入失败导致整个事务回滚。
- 同值负责人或优先级更新成功但不写事件；其他重复状态动作返回 `409`。

遵循“先锁工单，再操作其消息和事件”的顺序；不在持有事务时调用外部模型。数据库临时故障不伪装成业务冲突，不进行无界重试。

## 6. API 端点与协议

业务接口前缀统一为 `/api/v1`，以下路径省略前缀。

| 阶段 | 方法与路径 | 访问与用途 |
| --- | --- | --- |
| MVP | `POST /auth/register` | 公开，创建普通用户 |
| MVP | `POST /auth/login` | 公开，返回访问令牌 |
| MVP | `GET /users/me` | 已登录，当前用户信息 |
| MVP | `GET /admin/agents` | 管理员，分页查询可分配客服 |
| MVP | `POST /tickets` | 普通用户，创建工单 |
| MVP | `GET /tickets` | 按当前角色查询可见工单 |
| MVP | `GET /tickets/queue` | 客服，未分配队列摘要 |
| MVP | `GET /tickets/:id` | 可见工单详情 |
| MVP | `GET /tickets/:id/messages` | 可见工单的消息分页 |
| MVP | `POST /tickets/:id/messages` | 所属用户或当前负责客服回复 |
| MVP | `GET /tickets/:id/events` | 可见工单的事件分页 |
| MVP | `POST /tickets/:id/claim` | 客服认领 |
| MVP | `PUT /tickets/:id/assignee` | 管理员首次分配或转派 |
| MVP | `PATCH /tickets/:id/priority` | 当前负责客服或管理员调整优先级 |
| MVP | `POST /tickets/:id/resolve` | 当前负责客服解决 |
| MVP | `POST /tickets/:id/reopen` | 所属用户重开 |
| MVP | `POST /tickets/:id/close` | 所属用户或管理员关闭 |
| AI | `POST /tickets/:id/ai-runs` | 当前负责客服触发一次分析 |
| AI | `GET /tickets/:id/ai-runs/:run_id` | 当前负责客服或管理员读取结果 |
| 增强 | `GET /admin/ticket-stats` | 管理员按状态查看基础计数 |

独立健康检查不加业务前缀：`GET /healthz` 检查进程存活；`GET /readyz` 检查数据库可用性，失败返回 `503`。M1 只提供存活检查，M2 增加就绪检查。

### 6.1 输入与输出

- 使用 JSON；Bearer Token 放在 `Authorization` 请求头。
- 注册接收 `email/password/display_name`；登录接收 `email/password`。
- 登录成功返回 `access_token/token_type/expires_in`。
- 创建工单接收 `title/description/category`，用户 ID、负责人、状态、优先级由服务端决定。
- 回复接收 `content`；分配接收 `assignee_id`；优先级更新接收 `priority`。
- 认领、解决、重开、关闭不接受客户端指定的目标状态或操作者。
- 成功创建用户、工单、消息返回 `201`；查询和其他成功动作返回 `200`，动作返回更新后的工单 DTO。
- 错误统一为 `{code, message, request_id}`；原始 SQL、堆栈和上游密钥不返回客户端。

### 6.2 校验与列表

- 标题 1～120 字符，描述 1～10000 字符，回复 1～5000 字符；拒绝全空白文本，按 Unicode 字符数校验，不按 UTF-8 字节数计算正文长度。
- 显示名 1～64 字符；邮箱长度不超过 254 字符且格式有效。
- 枚举非法、ID 非正整数、分页不合法均返回 `400`。
- 分页参数为 `page/page_size`，默认 1/20，`page_size` 最大 100；超出范围返回 `400`。
- 列表返回 `items/total/page/page_size`；空列表返回空数组。
- 工单列表支持 `status/category`；管理员另支持 `assignee_id`，查询未分配使用 `unassigned=true`；二者不可同时指定。
- 客服队列仅支持分类和分页，固定查询 `open` 且负责人为空。
- 工单按 `created_at DESC, id DESC` 排序，消息、事件按 `id ASC` 排序；相同时间通过 ID 保证顺序确定。MVP 使用偏移分页，不承诺并发插入下的跨页快照。
- 静态 `/tickets/queue` 与动态工单路由同时注册并测试，避免被错误解释为工单 ID。

### 6.3 错误语义

| HTTP 状态 | 语义 |
| --- | --- |
| `400` | 输入、枚举、分页或标识格式错误 |
| `401` | 未登录、令牌无效或过期、登录凭据错误 |
| `403` | 角色不能使用此操作，或对可见资源没有动作权限 |
| `404` | 工单或子资源不存在，或当前账号不可见 |
| `409` | 邮箱已存在、认领竞争失败或状态不满足动作条件 |
| `500` | 未预期内部错误 |
| `502/504` | AI 阶段的上游响应异常或超时 |
| `429/503` | 增强阶段限流触发或 Redis 不可用；就绪检查失败也使用 `503` |

先通过认证和角色入口检查，再校验工单可见性，最后校验动作和状态；不向无权读取工单的人泄露其当前状态。

## 7. 第二阶段 AI 设计

### 7.1 最小调用流程

1. 校验当前账号为负责客服，工单处于 `in_progress`。
2. 短事务读取一致的工单与消息快照，保存 `running` 调用记录并提交。
3. 在事务外调用一种供应商的模型协议，默认超时 20 秒，不自动重试。
4. 校验结构化结果，保存 `succeeded/failed` 和结果或错误类别。
5. 返回调用 ID 和结果；查询接口重新按当前归属授权，调用者不因历史发起行为永久获得权限。

这是同步接口，不引入后台队列或异步轮询任务框架。调用期间的记录用于审计与失败诊断，不提供恢复或重试任务 API。持久化更新失败时返回内部错误，不把未落库结果描述为已保存。

### 7.2 数据与输入快照

`ai_runs` 主要字段为：`id/ticket_id/requested_by/status/provider/model/input_snapshot_id/input_truncated/result/error_category/created_at/finished_at`。新增表使用独立迁移。

- 一次生成建议分类、摘要和回复建议；分类必须是既定枚举，摘要上限 1000 字符，回复建议上限 5000 字符。
- 输入包含初始标题、描述及最近最多 20 条消息，按时间顺序组织，总输入最多 12000 字符；超限优先保留初始问题和最新消息，并标记截断。
- 输入快照标识使用工单相关输入字段及最新消息 ID 的确定性摘要；保存输入构建规则版本，不单靠秒级 `updated_at` 判断过时。
- 查询时重新比较快照标识，返回 `is_stale`；输入或最新消息变化后旧结果应标记过时。
- 配置从环境变量读取 API 地址、模型名、API Key；不把供应商错误正文直接持久化或返回客户端。
- 不主动发送账号邮箱、密码哈希或令牌。用户正文可能含个人信息，演示默认使用脱敏样例，不承诺自动识别所有敏感内容。
- 工单和消息作为不可信数据传入模型，不接受其改变系统指令、调用工具或自动执行动作。
- 模型失败不回滚或修改工单，不自动分类、分配或发送回复。

## 8. 后端增强与验证原则

Redis 首次仅用于登录限流，达到阈值返回 `429`，Redis 不可用时登录返回 `503`，不影响已登录用户的 MySQL 工单流程。代理与来源 IP 的信任配置、限流阈值及原子计数测试在 B1 定义和验证。

M1 起就提供基本请求 ID 和不记录敏感正文的日志；B3 完善优雅停机及全链路超时，不把基础错误处理推迟到最后。

单元测试覆盖状态规则和权限，`httptest` 覆盖 HTTP 绑定、状态码及认证，真实 MySQL 测试覆盖唯一约束、外键、事务回滚和并发认领。SQLite 不替代 MySQL 的事务与锁语义验证。

参考资料：

- [GORM Transactions](https://gorm.io/docs/transactions.html)：事务内一致使用事务连接。
- [GORM Migration](https://gorm.io/docs/migration.html)：了解 AutoMigrate 的能力和限制，本项目选择显式编号 SQL。
- [Go bcrypt](https://pkg.go.dev/golang.org/x/crypto/bcrypt)：密码哈希、比较及长度约束。
