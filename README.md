# SupportFlow AI

面向 Go 后端学习与实习求职展示的客服工单平台。**M1～M9 及补充任务 M5.1 已通过验收。** 用户可以提交、回复、重开和关闭自己的工单，客服可以认领、沟通和解决工单，管理员可以分配、转派和关闭已解决工单。

## M9 工单处理闭环

按下文在 PowerShell 导入本地环境、启动 MySQL，确认 `go run ./cmd/migrate version` 显示版本 4、`dirty=false`，再运行 `go run ./cmd/api`。在 Postman 中将 `base` 设为 `http://localhost:8080/api/v1`，分别登录用户、客服和管理员，使用各自的 Bearer Token。

1. 用户向 `POST {{base}}/tickets` 提交 `{"title":"无法登录","description":"重置密码后仍提示失败"}`，将响应中的工单 ID 保存为 `ticketId`。以下路径均接在 `{{base}}/tickets/{{ticketId}}` 后。
2. 客服向 `/claim` 发送 POST，Body 选 none。用户和客服依次向 `/messages` POST 一条 `{"content":"已重试一次"}`，各得 201。
3. 客服向 `/priority` 发送 PATCH，raw JSON 为 `{"priority":"high"}`，再向 `/resolve` 发送空 POST。两步均返回 200 和工单详情，状态变为 `resolved`。此时再次回复得到 409。
4. 用户向 `/reopen` 发送空 POST，确认状态回到 `in_progress` 且负责人相同。双方可继续回复。客服再次解决后，用户向 `/close` 发送空 POST，确认状态为 `closed`。管理员也可用另一张已解决工单验证关闭权限。
5. 用户读取 `GET /events`，按 ID 顺序核对优先级和状态的前后值。只有处理中的工单可调整优先级，重复设置当前值时仍返回 200，比较前后的 `updated_at` 和事件总数应相同。

配置独立空测试库后，运行 `go test -tags=integration -count=1 -v -run TestLifecycleLiveMySQL ./tests/integration` 可复现以上闭环、九组并发请求和数据库故障回滚。[M9 验收记录](docs/ROADMAP.md#m9-处理闭环)

## M8 工单沟通

在 PowerShell 中按下文导入本地 `.env`，启动 Compose MySQL。执行 `go run ./cmd/migrate up` 和 `go run ./cmd/migrate version`，确认版本为 4、`dirty=false` 后运行 `go run ./cmd/api`。

两个接口共用路径 `/api/v1/tickets/:id/messages`。POST 只接受 raw JSON `{"content":"重启后仍然出现这个问题。"}`。正文允许 1～5000 个 Unicode 字符，须包含非空白字符。成功返回 201 及 `id`、`ticket_id`、`author_id`、`content`、`created_at`。GET 按消息 ID 升序返回 `items/total/page/page_size`，默认第 1 页、每页 20 条，最多 100 条。

**Postman 验收**

1. 以普通用户创建工单，将响应的 `id` 保存为 `ticketId`。用同一令牌向 `/api/v1/tickets/{{ticketId}}/messages` 发送 POST，预期 201。未认领的客服 A 此时 GET 消息应为 404。
2. 用客服 A 向该工单的 `/claim` 发送 POST。切换用户与 A 的令牌分别发送一条消息，用 `GET /messages?page=2&page_size=2`（接在工单路径后）查看第三条，再从 `/events` 核对 `message_added` 中的 `message_id`。
3. 用管理员令牌 GET 消息，预期 200；用管理员 POST 回复，预期 403。向该工单的 `/assignee` 发送 PUT 分配给客服 B，A 的后续 GET 和 POST 均应为 404，B 可以读取历史并回复。

配置好独立空测试库后，运行 `go test -tags=integration -count=1 -v -run TestMessagesLiveMySQL ./tests/integration` 可复现真实网络验收。该测试启动 API、验证两种行锁顺序，并在测试库强制两次 INSERT 分别失败后核对消息与事件数量。完整回归结果见 [M8 验收记录](docs/ROADMAP.md#m8-工单沟通)。

## M7 认领、分配与转派

在 Postman 中分别用普通用户、客服 A、客服 B 和管理员登录，保存各自的 Bearer Token。以下路径以 `/api/v1` 为前缀。

1. 普通用户向 `POST /tickets` 提交两张工单。客服 A 从 `GET /tickets/queue` 取得编号，此时读取详情返回 404。
2. 客服 A 向第一张工单的 `POST /tickets/:id/claim` 发送空请求体。预期响应为 200，`status` 为 `in_progress`，`assignee_id` 为 A 的用户 ID。此时 A 可以读取详情和事件，重复认领返回 409。
3. 管理员从 `GET /admin/agents` 取得两个客服的数字 ID。向第二张工单的 `PUT /tickets/:id/assignee` 发送 raw JSON `{"assignee_id": {{agentAId}}}`，首次分配后在事件列表中可以看到 `assigned`。
4. 管理员对同一工单改发 B 的 ID，响应详情中的负责人变为 B，事件列表新增 `reassigned`。再次发送 B 的 ID 仍返回 200，比较两次详情的 `updated_at` 和事件总数，两者应相同。
5. 用 A 和 B 的令牌分别读取第二张工单的详情及 `GET /tickets/:id/events`，预期 A 得到 404，B 得到 200。普通用户调用两个写接口均返回 403，未登录调用返回 401，无效分配目标返回 400。

按下文配置空的专用测试库后，运行 `go test -tags=integration -count=1 -v -run TestAssignmentLiveMySQL ./tests/integration` 可单独复现 M7 网络验收。测试启动实际 API 进程，等两个 HTTP 请求都到达 MySQL 工单行锁后再释放测试锁，并核对 HTTP 结果、最终负责人和事件。测试还强制事件插入失败，核对工单和历史均已回滚。

## M5.1 管理员创建工作人员（M6 后补充）

首个管理员通过下文 M5 的 CLI 初始化。随后管理员可在 Postman 登录 `POST /api/v1/auth/login`，将返回的 access_token 填入 Bearer Token，再向 `POST /api/v1/admin/staff` 发送 raw JSON。示例中的密码变量请在本地填写。

```json
{"email":"agent@example.test","password":"{{staff_password}}","display_name":"客服小王","role":"agent"}
```

创建管理员时使用新邮箱并将 role 改为 admin。成功返回 201 和 id、email、display_name、role、created_at。随后用新账号登录验证：客服应出现在管理员的 `GET /api/v1/admin/agents` 列表，新管理员可用自己的令牌访问该接口。创建者由当前认证身份决定，发送 actor_id、id 或其他额外字段均返回 400。

匿名或令牌无效返回 401，普通用户、客服和已降权管理员返回 403，非法角色或输入返回 400，重复邮箱返回 409。成功操作以 request_id 关联操作者、新账号和角色。CLI 仍可用于本地恢复；公开注册只创建普通用户。

2026-11-22 的真实验收在专用空 MySQL 库启动 API 进程，覆盖两种角色的创建与登录、客服列表、降权、唯一约束冲突、数据库写入失败和审计元数据。按文末集成测试命令可重现。迁移版本为 3。

## M6 工单提交与查询

在项目根目录按下文导入环境变量，启动 MySQL，运行 `go run ./cmd/migrate up`、`go run ./cmd/migrate version`，确认已达到当前版本且 dirty=false，再运行 `go run ./cmd/api`。

在 Postman 中使用本地环境变量 `base=http://localhost:8080/api/v1`，分别登录普通用户、客服和管理员。以下请求均使用对应的 Bearer Token。

| 方法与路径（基于 `{{base}}`） | 验收方法 |
|---|---|
| POST /tickets | 普通用户以 raw JSON 提交 `{"title":"无法登录","description":"重置密码后仍提示失败","category":"account"}`，得到 201，保存响应中的 id。省略 category 时为 general。 |
| GET /tickets | 用 `?status=open&category=account&page=1&page_size=1` 检查 items 和 total。管理员可加 `unassigned=true` 或单独指定 `assignee_id`。 |
| GET /tickets/queue | 切换客服令牌。未分配摘要只有 id、title、category、priority、created_at，支持分类和分页。 |
| GET /tickets/:id | 将 :id 替换为创建时的编号。创建人和管理员得到 200，其他用户和未负责的客服得到 404。 |
| GET /tickets/:id/events | 同样的可见范围，创建人可看到 created 事件、操作者和初始状态。 |

创建接口只使用 title、description、category。身份和业务状态字段由服务端填写。列表默认第 1 页、每页 20 条，最大 100。没有匹配行时 items 为 []。工单按创建时间、ID 倒序，事件按 ID 升序。COUNT 与分页读取是独立查询，并发写入时不同页可能发生移动。

运行 `go test -tags=integration -count=1 -v ./tests/integration` 可复现实测。工单测试会在已核验的专用空库启动实际 API，通过 HTTP 验证五个端点，并用测试库 CHECK 约束拒绝一条事件写入，验证工单回滚。开发库仅执行 up。

数据库由 Compose 管理，API 和工作人员 CLI 共用 GORM 与编号 SQL 迁移。数据库不可用时，API 的 `/healthz` 返回 200，`/readyz` 返回 503。

## M5 工作人员初始化与客服列表

在项目根目录，按下文 PowerShell 步骤导入数据库环境变量并执行迁移。运行 CLI 的操作者需要能使用该数据库账号。每次执行只创建一条记录，邮箱已存在时命令以非零状态退出。

```powershell
$staffInput = Read-Host '输入新账号密码' -AsSecureString
try {
    $env:STAFF_PASSWORD = [PSCredential]::new('staff', $staffInput).GetNetworkCredential().Password
    go run ./cmd/admin -role admin -email admin@example.test -display-name 'Demo Admin'
} finally {
    Remove-Item Env:STAFF_PASSWORD -ErrorAction SilentlyContinue
    $staffInput.Dispose()
}
```

创建客服时将角色改为 `agent`，另选邮箱并重新输入密码。邮箱与普通注册使用同一规范化规则，密码按原样校验 8～72 字节。CLI 从进程环境读取密码，仅写入 bcrypt 哈希。

**Postman 验证**　启动 API，向 `POST /api/v1/auth/login` 提交该工作人员的邮箱和密码，再用返回的 access_token 作为 Bearer Token 调用 `GET /api/v1/admin/agents?page=1&page_size=1`。管理员得到 200，响应包含按 ID 升序排列的 items、客服总数 total、page 和 page_size。默认每页 20，允许 1～100，超出末页返回空数组，非法分页返回 400。换为客服或普通用户令牌得到 403，清空令牌得到 401。登录时请在本机使用凭据，避免同步密码及令牌。

**2026-11-22 实测**　独立空 MySQL 库内运行真实 CLI，验证两种工作人员角色、bcrypt、重复邮箱保护、非法输入、角色过滤、分页与同一令牌降权。开发库另创建一名管理员和两名客服，在真实 API 进程上验证管理员 200（total=2）、两名客服均 403、匿名 401。普通测试、集成测试、格式检查、vet 和两个入口构建均通过。

## 环境与依赖

- Go 1.27.0 或兼容的更新版本（与当前 `go.mod` 一致）。
- Git；完整验收 M2 需要 Docker Desktop + Compose 及 MySQL 8.4。未安装 Docker 时不要把纯 Go 检查当作完整验收。
- Gin v1.12.0 用于 HTTP；GORM v1.31.2、GORM MySQL 驱动 v1.6.0 和底层 `go-sql-driver/mysql` v1.8.1 用于连接；golang-migrate v4.20.1 用于独立迁移命令。配置、日志和测试使用标准库。
- `golang.org/x/crypto` v0.53.0 的 bcrypt 用于密码哈希；沿用已有版本，M3 将其列为直接依赖。
- `github.com/golang-jwt/jwt/v5` v5.3.1 用于 M4 的 HS256 签发与验证，版本已锁定。
- 正式模块路径为 `github.com/Planck-code/SupportFlow-AI`，与 GitHub 仓库保持一致。

规划文档：[需求](docs/PRD.md)、[架构](docs/ARCHITECTURE.md)、[路线](docs/ROADMAP.md)。

## VS Code F5 启动（本地开发推荐）

用 VS Code 打开项目根目录 `SupportFlow AI`，安装 Go 扩展（`golang.go`）。首次使用时，按下文创建并填写根目录 `.env`，启动 MySQL 并运行数据库迁移。

日常启动：

1. 确保 Docker Desktop 已启动；若数据库未运行，在项目根目录终端执行 `docker compose up -d --wait mysql`。
2. 按 `F5`，选择 `SupportFlow AI: 启动 API`（只有一个启动配置时可能直接启动）。如果 Go 扩展提示缺少 Delve（`dlv`），按提示安装调试工具后重试。
3. 打开 `http://localhost:8080/healthz` 和 `http://localhost:8080/readyz`，确认均返回 200 和 `{"status":"ok"}`。若 `.env` 修改了 `PORT`，使用对应端口。
4. 按 `Shift+F5` 停止调试。

`.vscode/launch.json` 将启动包固定为 `cmd/api`、工作目录固定为项目根目录，并通过 `envFile` 自动将 `.env` 注入 Go 进程；无需手动导入环境变量。配置不包含密码，也不修改 Go 程序的配置读取逻辑。参考 [Go 扩展调试文档](https://github.com/golang/vscode-go/blob/master/docs/debugging.md)。

**Run Code 启动 API**：用 VS Code 打开项目根目录，确保 MySQL 已启动，打开 `cmd/api/main.go` 并点击 **Run Code**。工作区的 Code Runner 配置会调用 `scripts/run-go.ps1`，从根目录读取 `.env` 后运行当前文件所在的 Go 包。修改配置后在终端按 Ctrl+C，再点 Run Code 即可。若已有 F5 启动的实例，先按 Shift+F5 停止，避免端口冲突。

## Windows PowerShell 启动

在项目根目录运行。下面将依赖、构建缓存和 Go 临时文件放到项目内；这些目录已被 Git 忽略。环境变量只影响当前 PowerShell 会话，不使用 `go env -w` 修改全局配置。

```powershell
Set-Location 'E:\Agent\workspace\SupportFlow AI'

New-Item -ItemType Directory -Force -Path .cache/mod,.cache/build,.cache/tmp | Out-Null
$env:GOMODCACHE = Join-Path $PWD '.cache/mod'
$env:GOCACHE = Join-Path $PWD '.cache/build'
$env:GOTMPDIR = Join-Path $PWD '.cache/tmp'

go mod download
```

首次使用复制配置样例，再用编辑器在本地填写不同的 `DB_PASSWORD` 和 `MYSQL_ROOT_PASSWORD`，并按文末 M4 配置步骤生成 `JWT_SECRET`；不要提交 `.env`。已有 `.env` 时直接编辑，不覆盖原文件：

```powershell
if (-not (Test-Path .env)) { Copy-Item .env.example .env }
notepad .env
```

Compose 自动读取根目录 `.env`，Go 程序不自动读取。每个运行 API、迁移或集成测试的 PowerShell 窗口先执行以下代码导入配置（只处理本项目样例的单行 `KEY=value`，不要加引号、行尾注释或多行值）：

```powershell
Get-Content .env | ForEach-Object {
    if (-not [string]::IsNullOrWhiteSpace($_) -and -not $_.TrimStart().StartsWith('#')) {
        $parts = $_ -split '=', 2
        if ($parts.Count -ne 2) { throw '配置必须使用 KEY=value' }
        [Environment]::SetEnvironmentVariable($parts[0].Trim(), $parts[1], 'Process')
    }
}
```

然后按后文启动 MySQL、运行迁移，最后执行：

```powershell
go run ./cmd/api
```

默认监听 `:8080`，日志中的 `starting HTTP server` 表示开始尝试监听；是否成功以健康请求为准。启动失败会记录错误并以非零退出码退出。开发时使用 `Ctrl+C` 停止服务，完整的优雅停机留到 B3。

在第二个 PowerShell 窗口检查：

```powershell
$response = Invoke-WebRequest -Uri 'http://localhost:8080/healthz' -UseBasicParsing
$response.StatusCode
$response.Content
$response.Headers['X-Request-ID']
```

预期状态码 `200`，Content-Type 为 JSON，正文为：

```json
{"status":"ok"}
```

`X-Request-ID` 为每次请求新生成的标识，可与终端日志中的 `request_id` 对照。

## 端口配置

| 环境变量 | 默认值 | 约束 |
| --- | --- | --- |
| `PORT` | `8080`（未设置时） | 整数，范围 1～65535；显式空字符串或非法值报错 |

自定义端口，在启动服务的窗口运行：

```powershell
$env:PORT = '9090'
go run ./cmd/api
```

然后访问 `http://localhost:9090/healthz`。恢复默认配置：

```powershell
Remove-Item Env:PORT -ErrorAction SilentlyContinue
```

`.env.example` 仅提供配置说明。程序不会自动读取 `.env`；复制文件后还需在 PowerShell 导入环境变量，不额外引入 dotenv 库。

如端口占用，停止占用该端口的服务或换一个端口；不要通过忽略监听错误继续启动。

## 格式、测试与构建

在已设置上述缓存变量的项目根目录窗口运行：

```powershell
gofmt -w cmd internal tests
gofmt -l cmd internal tests
go test ./...
go vet ./...
go build ./cmd/api
go build -o .cache/migrate.exe ./cmd/migrate
```

`gofmt -l` 无输出代表没有待格式化文件；测试和检查应成功退出。Windows 构建产物为根目录的 `api.exe`，已被忽略，可运行：

```powershell
.\api.exe
```

需要查看具体测试项时使用 `go test -v ./...`。路由测试通过 `httptest` 直接发送请求，无需先启动服务；它不替代真实进程启动检查。

测试覆盖：

- 默认、自定义和端口边界；空值、非法值与越界配置。
- 健康接口的状态码、精确 JSON 正文和 Content-Type。
- 请求 ID 生成与日志关联，拒绝沿用客户端传入的请求 ID。
- 请求日志不含 Authorization、查询参数、原始未匹配路径或正文。
- 数据库配置、带特殊字符密码的 DSN 往返解析、连接池上限和不可用数据库的 Ping 失败路径。
- readiness 成功、失败及恢复；错误响应不泄露数据库详情；迁移源文件可读取。
- 注册必填字段、JSON 类型、邮箱/显示名规范化、Unicode 显示名上限、8～72 字节密码、role 字段拒绝、错误状态与 DTO 脱敏。

这些测试无需 MySQL。真实迁移与会话参数测试通过 `integration` 构建标签单独运行，见后文。

## 代码结构与学习入口

```text
cmd/api/main.go                 日志、配置、路由与 HTTP 服务组装
internal/config/               从进程环境加载并校验端口
internal/auth/                 JWT HS256 签发与严格验证
internal/database/             GORM 连接池与 golang-migrate 初始化
cmd/migrate/                   独立 up/version 命令，不接受命令行密码
internal/router/               注册中间件、GET /healthz 和 GET /readyz
internal/handler/health.go      输出健康 JSON
internal/handler/ready.go       有界数据库 Ping 与就绪 JSON
internal/middleware/           请求 ID 与结构化访问日志
internal/model/                users 数据库模型
internal/dto/                  注册/登录请求、令牌响应、安全用户响应
internal/service/              注册/登录规则、bcrypt、服务端固定注册角色
internal/repository/           带 context 的 GORM Create、邮箱/ID 查询与错误映射
migrations/                    000001 基线、000002 users 表
tests/integration/             显式启用的真实 MySQL 验证
```

启动顺序：`main` 创建日志 → 加载 HTTP、JWT 和数据库配置 → 创建 JWT 组件与 GORM 连接池 → 初次限时 Ping（失败只警告）→ 手动组装 Repository、注册/登录 Service、认证中间件及路由 → HTTP 监听。缺少有效 JWT_SECRET 时拒绝启动；迁移命令独立运行，不要求 JWT_SECRET。连接失败后无需重启 API，后续 readiness 会重新 Ping。

请求顺序：HTTP Server → Gin → 请求日志中间件生成 ID → Recovery 中间件 → `handler.Healthz` → `c.JSON` 写入状态码和正文 → 中间件记录最终状态与耗时。

Recovery 使用受限日志，不输出默认的请求头或 panic 详情。M3 为实际注册业务增加 Service、Repository、DTO 和 Model，没有通用 CRUD、BaseService 或依赖注入框架。

优先理解：Go 包与导入路径、显式错误处理、环境变量、Gin 的 `HandlerFunc` 与 `Context`、`c.Next()` 的前后执行顺序，以及 Gin 如何实现标准库的 `http.Handler` 接口。参考 [Gin 官方入门](https://gin-gonic.com/en/docs/quickstart/)。

## Git 提交规范

已初始化本地仓库，`origin` 指向 <https://github.com/Planck-code/SupportFlow-AI.git>。每个 ROADMAP 任务独立验收后提交并推送当前分支，不创建额外分支、不强制推送；未通过实际验收的任务保留未提交工作。提交信息保留 Conventional Commits 前缀，描述使用中文。

检查代码与测试结果后，可手动执行：

```powershell
git status --short
git add .
git diff --cached --stat
git commit -m 'feat: 实现用户登录与 JWT 身份认证'
$currentBranch = git branch --show-current
git push -u origin $currentBranch
```

提交前确认 `.env`、缓存和二进制文件未进入暂存区。推送需要本机 GitHub 权限和可用网络；若失败，先检查网络、代理及认证，不把远程地址配置成功视为推送成功。

## M2：启动 MySQL

先确认 Docker Desktop 正在运行，且 `docker` 已在当前终端的 PATH 中。安装后旧终端可能需要重开；也可仅在当前 PowerShell 会话临时追加实际安装目录，不修改系统 PATH：

```powershell
# 仅适用于当前用户目录安装的 Docker Desktop；其他位置按实际路径调整。
$dockerBin = Join-Path $env:LOCALAPPDATA 'Programs\DockerDesktop\resources\bin'
if (Test-Path (Join-Path $dockerBin 'docker.exe')) { $env:PATH = "$dockerBin;$env:PATH" }
```

然后检查并启动：

```powershell
docker version
docker compose version
docker compose config --quiet
docker compose up -d --wait mysql
docker compose ps
```

Compose 锁定 `mysql:8.4.13`，仅向本机 `127.0.0.1` 暴露数据库端口。使用 `mysql_data` 命名卷持久化数据，初始化非 root 的 `DB_USER`。healthcheck 使用该用户执行 `SELECT 1`，而非仅检查进程。

如果宿主机 3306 已被占用或无法绑定，在本项目 `.env` 中将 `DB_PORT` 改为一个可用端口，例如 `13306`，再重新导入环境变量并运行 `docker compose up -d --wait mysql`。容器内端口仍为 3306；Compose 映射和本地 Go 连接会同时使用 `DB_PORT`。本次验收使用 13306，未停止占用 3306 的其他服务。

`DB_PASSWORD` 和 `MYSQL_ROOT_PASSWORD` 必须在本地填写；不要运行不带 `--quiet` 的配置展开并分享结果，以免泄露密码。镜像初始化变量仅对空数据卷生效，修改 `.env` 不会自动修改已有数据库密码。常规停止用 `docker compose stop mysql`，恢复用 `docker compose start mysql`；不要为排错随意删除数据卷。

### 数据库配置

| 变量 | 默认值 / 要求 |
| --- | --- |
| `DB_HOST` | `127.0.0.1`，本地 API 连接宿主机端口 |
| `DB_PORT` | `3306`，1～65535，同时决定 Compose 映射端口 |
| `DB_USER` | `supportflow`，不使用 root 运行 API |
| `DB_PASSWORD` | 必填，不提供默认密码 |
| `DB_NAME` | `supportflow` |
| `DB_TIMEOUT` | `3s`，连接、读和写的驱动超时，必须为正时长 |
| `DB_MAX_OPEN_CONNS` | `10`，1～1000 |
| `DB_MAX_IDLE_CONNS` | `5`，0～最大连接数 |
| `DB_CONN_MAX_LIFETIME` | `30m`，必须为正时长 |
| `MYSQL_ROOT_PASSWORD` | Compose 初始化必填，API 不读取 |

显式空值不使用默认值。MySQL DSN 由驱动构造，不手工拼接密码；连接启用 `parseTime`、UTC、utf8mb4。API 不允许多语句，迁移使用独立连接池开启多语句。数据库暂不可用时仍保留存活检查，缺少必要配置则启动失败。

### 编号 SQL 迁移

在项目根目录、已导入环境变量的窗口执行：

```powershell
go run ./cmd/migrate up
go run ./cmd/migrate version
```

这是基于官方 golang-migrate 引擎的项目内 CLI，复用数据库环境变量，避免把带密码的 DSN 放进命令行。无需全局安装迁移工具。它与 API 分开运行，不使用 `AutoMigrate`。

`000001_baseline` 建立迁移基线，`000002_create_users` 创建 users 表，`000003_tickets` 创建 tickets 和 ticket_events，`000004_ticket_messages` 创建消息表。完整 up 后当前预期版本 **4** 且 `dirty=false`。版本由 schema_migrations 记录。

未来按编号增加迁移，不修改已应用文件。若 dirty，停止重试并检查迁移 SQL 和数据库实际状态；本 CLI 不提供 force 或生产回滚命令。迁移失败只输出不含 DSN 的提示，不将可能含业务数据的数据库错误原文写入日志。

### 健康与就绪

API 启动后，在另一个窗口执行：

```powershell
Invoke-WebRequest http://localhost:8080/healthz -UseBasicParsing
Invoke-WebRequest http://localhost:8080/readyz -UseBasicParsing
```

健康检查始终只判断进程响应。readiness 对当前连接池执行最长 2 秒的 `PingContext`：成功返回 200 和 `{"status":"ok"}`；失败返回 503、`database_unavailable` 和请求 ID，不返回底层错误。它不检查是否已迁移，因此部署时仍必须单独执行迁移步骤。

可在 API 保持运行时停止 MySQL，预期 `/healthz` 仍为 200、`/readyz` 变为 503；再启动 MySQL并等待就绪，预期 readiness 恢复 200。PowerShell 的 `Invoke-WebRequest` 对 503 抛异常，可在 catch 中查看 `$_.Exception.Response.StatusCode`；PowerShell 7 也可加 `-SkipHttpErrorCheck`。

### 真实 MySQL 集成测试

测试只接受名字以 `_test` 结尾、与应用库不同且完全空的专用数据库；拒绝修改已有表。先用 root 的交互式密码输入创建测试库并授权（假设 `DB_USER=supportflow`）：

```powershell
docker compose exec mysql mysql -uroot -p --execute "CREATE DATABASE IF NOT EXISTS supportflow_m2_test CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci; GRANT ALL PRIVILEGES ON supportflow_m2_test.* TO 'supportflow'@'%';"
$env:TEST_DATABASE_NAME = 'supportflow_m2_test'
go test -tags=integration -count=1 -v ./tests/integration
```

若自定义了 `DB_USER`，同步修改授权语句。测试使用应用的连接配置，将库名改为 `TEST_DATABASE_NAME`。确认选中的是专用空库后执行迁移。每个测试结束按 ticket_messages → ticket_events → tickets → users → schema_migrations 顺序清理本次拥有的表。请独占测试库运行整套测试。

显式运行集成测试而未配置数据库时会失败，不静默跳过；普通 `go test ./...` 不包含带 `integration` 标签的测试，普通测试通过不代表真实 MySQL 验收通过。这里具体采用 `TEST_DATABASE_NAME` 配合 `DB_*`，无需额外填写带密码的测试 DSN。

## M2 历史验收

2026-11-02，Windows 11 + WSL 2，Docker Engine 29.9.2、Compose v5.6.1、Context `desktop-linux`。使用现有 Docker Desktop，没有安装软件或修改系统配置。本地 `.env` 已存在，保留其密码，仅调整冲突的数据库映射端口；该文件被 Git 忽略。

实际执行并通过：

- MySQL 8.4.13 容器 healthy，应用非 root 账号连接成功，应用与 Compose 的用户名、库名及密码配置一致。
- 开发库显式 up、版本查询及重复 up：版本 1，`dirty=false`；没有在开发库执行 down。
- 专用空库 `supportflow_m2_test` 的真实集成测试：连接、GORM SQL、UTC/utf8mb4、up、重复 up、down 和再次 up 均通过，测试只清理该测试库的迁移元数据。
- 同一个 API 进程：数据库正常时 healthz=200、readyz=200；停止本项目 MySQL 时 healthz=200、readyz=503；恢复后 healthz=200、readyz=200。没有重启 API，也没有操作其他数据库服务。
- gofmt、`go test -count=1 ./...`、`go vet ./...`、`go build ./cmd/api`、`go build ./cmd/migrate`。

验收后 API 验证进程已停止，MySQL 容器保留 healthy，数据卷未删除。M2 已满足提交条件；后续开发必须另行启动 M3，不顺带实施。

## M3：普通用户注册

先按上文导入 `.env`，启动 MySQL，执行 `go run ./cmd/migrate up` 和 `go run ./cmd/api`。开发库原版本 1 可直接向上升级至 2，不需要清库。

### Postman 操作

1. 新建 HTTP 请求，方法 `POST`，URL 为 `http://localhost:8080/api/v1/auth/register`。
2. Authorization 选择 No Auth；Body 选择 raw / JSON，Content-Type 为 `application/json`。
3. 输入以下演示内容（仅为接口示例，不是数据库密码）：

```json
{
  "email": "test@example.com",
  "password": "12345678",
  "display_name": "Test User"
}
```

成功为 HTTP 201，仅返回 `id/email/display_name/role/created_at`，role 固定 `user`。重复发送同一邮箱或其大小写变体返回 409。错误响应为 `{code,message,request_id}`，可通过响应头 `X-Request-ID` 与服务端日志关联。

可依次尝试：邮箱加空格和改为大写（会先规范化）；空白显示名、短密码、超过 72 字节密码（400）；添加 `"role":"admin"` 或 `"role":null`（400）。注册接口本身不返回 JWT，也不需要预先获取令牌；注册后通过登录接口获取令牌。

### 注册规则与执行流程

- Handler 用 `ShouldBindJSON` 处理 JSON/必填字段并明确拒绝 role；不返回原始绑定错误。
- Service 对邮箱 TrimSpace + ToLower 后用标准库 `net/mail` 验证，邮箱至多 254 字节；不接受带显示名的 `Name <email>` 写法。display_name 去除首尾空白，限制 1～64 个 Unicode 字符。
- 密码不 trim，以 UTF-8 字节数限制为 8～72，使用 bcrypt DefaultCost（10）生成带随机盐的哈希。24 个三字节汉字等于 72 字节，25 个就超限。
- Repository 只调用 `db.WithContext(ctx).Create(user)`，不先查邮箱；MySQL UNIQUE 负责最终一致性，1062 或 GORM 重复键错误映射为冲突；其他存储错误统一返回 500，不向响应或日志输出底层 SQL、密码或哈希。
- users.email 延续项目的 `utf8mb4_0900_ai_ci` 排序规则，唯一比较不区分大小写和重音；不承诺区分该排序规则认为等价的 Unicode 地址。role 列使用二进制排序规则并 CHECK 三种合法值，公开注册始终只写 `user`。
- GORM 的 NowFunc 显式使用 UTC，匹配数据库会话 UTC 与 `DATETIME(6)`。响应时间输出 RFC 3339。

调用流程：`main` 复用 GORM 连接 → 构造 Users Repository → 构造 Registration Service → Router 注册 Handler → 请求校验与规范化 → bcrypt → MySQL INSERT → 安全 DTO 响应。

Model 表达持久化字段，包含内部 password_hash；请求和响应 DTO 表达 HTTP 契约，响应采用字段白名单，不直接序列化 Model。Service 管业务，Repository 管数据库及错误码。服务仅定义一个 Create 能力接口、Handler 仅定义 Register 能力接口，供隔离测试使用，不建设通用接口框架。

重点阅读 `internal/handler/register.go`、`internal/service/registration.go`、`internal/repository/user.go`、`migrations/000002_create_users.up.sql` 和 `tests/integration/registration_test.go`。bcrypt 是密码哈希而非可逆加密；其验证函数可比较原密码与保存的哈希，不能使用明文或普通快速哈希代替。参考 [bcrypt 官方文档](https://pkg.go.dev/golang.org/x/crypto/bcrypt)。

### M3 真实验收

2026-11-02，沿用 MySQL 8.4.13、宿主端口 13306，实际通过：

- 全量普通单元/HTTP 测试、格式检查、vet、API 和迁移工具构建。
- 真实集成测试：注册行、密码非明文且 bcrypt 验证成功、大小写邮箱冲突、直接 Repository 插入大小写变体触发唯一约束、非法角色约束、取消的 context。
- 8 个并发 HTTP 注册请求共享一个邮箱：1 个 201、7 个 409，数据库恰好 1 行；没有用 mock 代替数据库唯一约束。
- 专用空库迁移 1 → 2 → 1 → 2、完整 down/up，且可再次执行测试；清理 users 和迁移元数据均限定在已核验的专用测试库。
- 启动真实 API 二进制连接专用测试库，通过网络 HTTP 验证 201/409/400/role=admin 被拒绝；核查真实数据库记录的邮箱、角色、bcrypt 格式和非明文属性。响应及请求日志不含密码或哈希。
- 开发库仅向上迁移到版本 2、dirty=false，没有写入注册测试用户。测试数据已清理，API 验证进程已停止，MySQL 保持 healthy。

重复检查：先按上文导入环境、确保专用测试库为空，再执行：

```powershell
gofmt -l cmd internal tests
go test -count=1 ./...
go vet ./...
go build ./cmd/api
go build ./cmd/migrate
go test -tags=integration -count=1 -v ./tests/integration
```

## M4：登录与 JWT 身份认证

### 本地密钥配置

API 要求 `JWT_SECRET` 至少 32 字节，没有默认值。首次配置时，在项目根目录执行下面代码，将随机值写入被忽略的 `.env`，不输出密钥；已有合格密钥则保留。此示例生成 48 个随机字节，再编码为 Base64 文本，服务端直接使用该文本作为签名密钥，不额外解码。

```powershell
$envPath = Join-Path $PWD '.env'
$localConfig = [IO.File]::ReadAllText($envPath)
if ($localConfig -notmatch '(?m)^JWT_SECRET=.{32,}\r?$') {
    $randomBytes = New-Object byte[] 48
    $rng = [Security.Cryptography.RandomNumberGenerator]::Create()
    $rng.GetBytes($randomBytes)
    $rng.Dispose()
    $jwtSecret = [Convert]::ToBase64String($randomBytes)
    if ($localConfig -match '(?m)^JWT_SECRET=.*$') {
        $localConfig = [regex]::Replace($localConfig, '(?m)^JWT_SECRET=.*$', ('JWT_SECRET=' + $jwtSecret))
    } else {
        $localConfig = $localConfig.TrimEnd() + [Environment]::NewLine + 'JWT_SECRET=' + $jwtSecret + [Environment]::NewLine
    }
    [IO.File]::WriteAllText($envPath, $localConfig, [Text.UTF8Encoding]::new($false))
}
```

然后重新执行前文的 PowerShell 环境变量导入代码，再 `go run ./cmd/api`。VS Code F5 会读取 `.env`，已有调试进程需要重启。不要将密钥放进命令行参数、示例文件或截图；更换密钥会使旧令牌失效。

### Postman 登录与当前用户

1. 先通过 M3 注册接口创建自己的本地演示用户。
2. `POST http://localhost:8080/api/v1/auth/login`，Authorization 选 No Auth，Body 选 raw / JSON，使用注册时的邮箱和密码：

```json
{"email":"test@example.com","password":"12345678"}
```

3. 成功返回 200，字段为 `access_token`、`token_type: "Bearer"`、`expires_in: 7200`（秒）。令牌不打印到服务器日志；请仅在本地保存使用。
4. 新建 `GET http://localhost:8080/api/v1/users/me`，Authorization 选择 Bearer Token，把登录结果中的 access_token 填入 Token 输入框（无需重复写 `Bearer`）。成功返回 200，只有 `id/email/display_name/role/created_at`。
5. 改为 No Auth 或修改令牌字符，应返回 401；改错登录密码，或使用不存在的邮箱，均返回相同的 401 错误。缺少邮箱/密码、错误 JSON 或非法邮箱返回 400。数据库查询故障返回 500，不能当作凭据错误。

客户端传入的 id/role 不参与身份判断。密码保留原样；非空但不满足注册密码字节范围的登录凭据返回 401，避免 bcrypt 的 72 字节截断问题。登录和 `/users/me` 成功响应设置 `Cache-Control: no-store`。退出仅清除客户端令牌，MVP 不提供刷新令牌、黑名单或服务端注销；原令牌在有效期内仍能通过认证。

### 代码执行与学习顺序

- 登录：`handler/login.go` 用 ShouldBindJSON 绑定 → `service/login.go` 复用注册邮箱规范化 → `repository/user.go` 按邮箱查询 → bcrypt.CompareHashAndPassword 比较 → `auth/jwt.go` 签发 → Handler 输出令牌 DTO。
- `NewLogin` 初始化一次随机 dummy hash，未知账号也执行默认成本 bcrypt 比较，减少账号存在性耗时差异；这不是用户密码或账号，不写入数据库。数据库错误和损坏的已存储哈希作为内部错误处理。
- JWT 由 header、payload、signature 三部分组成；签名防篡改，不加密 payload。只放 sub（十进制用户 ID）、iat、exp，固定签发 2 小时；不放邮箱、角色、密码或哈希。验证限制 HS256，要求 exp/iat，校验过期、未来签发时间、exp 晚于 iat，以及合法正整数 sub。
- `/users/me`：`middleware/authentication.go` 提取单个 Bearer Header → 验证 JWT → Repository 按 ID 重新查询 → 将安全 DTO 放进 Gin Context → `handler/me.go` 输出。用户删除后返回 401，角色/显示名变化后立即读取新值，数据库故障返回 500。M4 只有认证，不实现角色授权框架。
- 错误统一包含 code/message/request_id。认证失败中止后续 Handler；JWT 错误详情、数据库底层错误、Authorization 和请求体均不记录到日志。

优先阅读 `internal/service/login.go`、`internal/auth/jwt.go`、`internal/middleware/authentication.go` 和 `tests/integration/authentication_test.go`。接口只定义消费方实际需要的查询能力；JWT 使用具体组件，依赖在 main 显式组装。

参考：[JWT 验证选项](https://golang-jwt.github.io/jwt/usage/parse/)、[bcrypt 比较 API](https://pkg.go.dev/golang.org/x/crypto/bcrypt)。

### M4 验收记录（2026-11-04）

- 普通测试覆盖成功登录、规范化、同文案错误凭据、参数校验、哈希错误、JWT 声明、算法白名单、签名、过期、缺失/重复/错误 Authorization、无效用户、最新用户信息和日志脱敏。时间测试使用固定时钟或构造令牌，没有等待两小时。
- 真实 MySQL：复用独立空库 `supportflow_m2_test` 的安全检查/清理；注册后登录成功，修改测试用户的角色与显示名后 `/users/me` 返回最新值，删除该测试用户后旧令牌返回 401；独立关闭的真实 SQL 连接池使登录及已签名令牌认证返回 500。未停止开发数据库，也未删除开发库数据。
- 原有基线/users 迁移 up/down、版本 2 检查和 8 并发注册均保留并通过。M4 没有新增或修改 SQL 迁移，测试结束清理仅限本次拥有的测试库表。
- 实际启动 API 二进制，连接专用测试库，网络请求验证：注册 201 → 登录 200 → 带令牌 me 200 → 无令牌 me 401；错误密码 401，healthz/readyz 均 200。查验真实注册行与 bcrypt；API 日志不含实际密码、哈希、令牌、JWT 密钥或数据库密码。
- 实际 API 进程验证缺失和过短 JWT_SECRET 均拒绝启动；本地 `.env` 已生成随机密钥，未输出其值；`.env.example` 只保留空值。
- 格式检查、`go test -count=1 ./...`、`go vet ./...`、`go build ./cmd/api`、`go build ./cmd/migrate`、`go test -tags=integration -count=1 -v ./tests/integration` 全部实际执行通过。普通测试不替代数据库测试，MySQL 测试未跳过。
- 验证用 API 已停止，专用测试库已清理，MySQL 保留 healthy。M4 完成后停止，M5 尚未开始。
