# SupportFlow AI

面向 Go 后端学习与实习求职展示的客服工单平台。本次只实现 **M1：服务骨架**。

当前能力：Gin HTTP 服务、环境变量端口配置、JSON 结构化日志、请求 ID、`GET /healthz` 和自动化测试。尚未连接数据库，未实现注册登录、工单、JWT、Redis、AI 或前端；`/readyz` 留到 M2。

## 环境与依赖

- Go 1.27.0 或兼容的更新版本（与当前 `go.mod` 一致）。
- Git；当前无需 MySQL 或 Docker。
- 直接依赖：Gin v1.12.0，用于路由、中间件与 JSON 响应。配置、日志和测试使用 Go 标准库；`go.mod` 中其他模块是间接依赖，不代表引入了对应业务功能。
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

`.env.example` 仅提供配置说明。程序不会自动读取 `.env`，复制文件也不会使配置生效；M1 不额外引入 dotenv 库。

如端口占用，停止占用该端口的服务或换一个端口；不要通过忽略监听错误继续启动。

## 格式、测试与构建

在已设置上述缓存变量的项目根目录窗口运行：

```powershell
gofmt -w cmd internal
gofmt -l cmd internal
go test ./...
go vet ./...
go build ./cmd/api
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

## 代码结构与学习入口

```text
cmd/api/main.go                 日志、配置、路由与 HTTP 服务组装
internal/config/               从进程环境加载并校验端口
internal/router/               注册中间件与 GET /healthz
internal/handler/health.go      输出健康 JSON
internal/middleware/           请求 ID 与结构化访问日志
```

启动顺序：`main` 创建 `slog.Logger` → `config.Load` → `router.New` → `http.Server.ListenAndServe`。

请求顺序：HTTP Server → Gin → 请求日志中间件生成 ID → Recovery 中间件 → `handler.Healthz` → `c.JSON` 写入状态码和正文 → 中间件记录最终状态与耗时。

Recovery 使用受限日志，不输出默认的请求头或 panic 详情。M1 没有复杂业务，因此不创建 Service、Repository、DTO 等空层。

优先理解：Go 包与导入路径、显式错误处理、环境变量、Gin 的 `HandlerFunc` 与 `Context`、`c.Next()` 的前后执行顺序，以及 Gin 如何实现标准库的 `http.Handler` 接口。参考 [Gin 官方入门](https://gin-gonic.com/en/docs/quickstart/)。

## Git 提交规范

已初始化本地仓库，`origin` 指向 <https://github.com/Planck-code/SupportFlow-AI.git>。每个 ROADMAP 任务独立验收后提交并推送当前分支，不创建额外分支、不强制推送；未通过实际验收的任务保留未提交工作。提交信息保留 Conventional Commits 前缀，描述使用中文。

检查代码与测试结果后，可手动执行：

```powershell
git status --short
git add .
git diff --cached --stat
git commit -m 'feat: 初始化 SupportFlow AI 后端项目'
$currentBranch = git branch --show-current
git push -u origin $currentBranch
```

提交前确认 `.env`、缓存和二进制文件未进入暂存区。推送需要本机 GitHub 权限和可用网络；若失败，先检查网络、代理及认证，不把远程地址配置成功视为推送成功。
