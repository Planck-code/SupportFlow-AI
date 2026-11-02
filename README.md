# SupportFlow AI

面向 Go 后端学习与实习求职展示的客服工单平台。**M1～M3 已通过验收；当前实现普通用户注册，停止在 M3，不进入 M4 登录与 JWT。**

当前代码包含 Gin HTTP 服务、配置、JSON 日志、请求 ID、健康和就绪检查、GORM 连接池、Compose MySQL、显式 SQL 迁移及用户注册。数据库不可用时进程仍可运行，`/healthz` 返回 200，`/readyz` 返回 503。未开发登录、工单、JWT、Redis、AI 或前端。

## 环境与依赖

- Go 1.27.0 或兼容的更新版本（与当前 `go.mod` 一致）。
- Git；完整验收 M2 需要 Docker Desktop + Compose 及 MySQL 8.4。未安装 Docker 时不要把纯 Go 检查当作完整验收。
- Gin v1.12.0 用于 HTTP；GORM v1.31.2、GORM MySQL 驱动 v1.6.0 和底层 `go-sql-driver/mysql` v1.8.1 用于连接；golang-migrate v4.20.1 用于独立迁移命令。配置、日志和测试使用标准库。
- `golang.org/x/crypto` v0.53.0 的 bcrypt 用于密码哈希；沿用已有版本，M3 将其列为直接依赖。
- 正式模块路径为 `github.com/Planck-code/SupportFlow-AI`，与 GitHub 仓库保持一致。

规划文档：[需求](docs/PRD.md)、[架构](docs/ARCHITECTURE.md)、[路线](docs/ROADMAP.md)。

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

首次使用复制配置样例，再用编辑器在本地填写不同的 `DB_PASSWORD` 和 `MYSQL_ROOT_PASSWORD`，不要提交 `.env`。已有 `.env` 时直接编辑，不覆盖原文件：

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
internal/database/             GORM 连接池与 golang-migrate 初始化
cmd/migrate/                   独立 up/version 命令，不接受命令行密码
internal/router/               注册中间件、GET /healthz 和 GET /readyz
internal/handler/health.go      输出健康 JSON
internal/handler/ready.go       有界数据库 Ping 与就绪 JSON
internal/middleware/           请求 ID 与结构化访问日志
internal/model/                users 数据库模型
internal/dto/                  注册请求与安全用户响应
internal/service/              注册规则、bcrypt、服务端固定角色
internal/repository/           带 context 的 GORM Create 与重复键映射
migrations/                    000001 基线、000002 users 表
tests/integration/             显式启用的真实 MySQL 验证
```

启动顺序：`main` 创建日志 → 加载 HTTP 和数据库配置 → GORM 创建连接池 → 获取底层 `*sql.DB` 并设置连接池参数 → 初次限时 Ping（失败只警告）→ 组装路由 → HTTP 监听。连接失败后无需重启 API，后续 readiness 会重新 Ping。

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
git commit -m 'feat: 实现用户注册与密码安全存储'
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

`000001_baseline` 的 up/down 均只有 `SELECT 1`，用于建立迁移基线；M3 的 `000002_create_users` 创建 users 表、邮箱唯一约束和角色约束。完整 up 后当前预期版本 **2** 且 `dirty=false`，重复 up 应成功且不产生新版本。`schema_migrations` 由迁移引擎维护。未修改 000001，也不在开发库执行 down。

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

若自定义了 `DB_USER`，同步修改授权语句。测试使用应用的 host、port、user、password，只将库名改为 `TEST_DATABASE_NAME`，不使用 root。测试验证真实 Ping、GORM SQL、会话 utf8mb4/UTC、版本 1 基线、版本 2 users up/down、重复 up 和完整 down/up，再测试真实注册、bcrypt、唯一索引和 8 并发 HTTP 请求。每个测试先验证选中库名与空库，结束后仅清理本次拥有的 users 和 schema_migrations 表，保证下次可重复运行。不要同时在这个测试库执行其他迁移或业务操作；测试不使用 `t.Parallel`。

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

可依次尝试：邮箱加空格和改为大写（会先规范化）；空白显示名、短密码、超过 72 字节密码（400）；添加 `"role":"admin"` 或 `"role":null`（400）。没有登录或 JWT 返回值，不需要预先获取令牌。

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
