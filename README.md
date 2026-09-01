# AI Gateway Go V2

Go 后端现在位于独立的 `backend/`，采用与 `D:/vue-element-plus-admin/backend` 相同方向的模块化单体分层：DTO、Middleware、Auth/User Handler-Service-Repository、总 Router 和进程装配彼此分离。数据库访问使用 GORM；多个业务服务、gRPC、JWKS、OIDC 和 OAuth 均未引入。

## 仓库布局

```text
ai-gateway/
├─ backend/              # 完整 Go module 和 Dockerfile
│  ├─ cmd/
│  ├─ internal/
│  ├─ migrations/         # PostgreSQL 版本化 up/down migration
│  ├─ go.mod
│  └─ go.sum
├─ front/                # Vue 3 + Vite 管理界面
├─ docs/
├─ .env                  # 本地 Compose 配置，位于最外层且不提交
├─ .env.example          # 可提交的配置示例
└─ docker-compose.yml    # Gateway、API、migration job、PostgreSQL、Redis
```

仓库不再包含 K8s 或旧部署目录。

## 当前认证与授权能力

- GORM + PostgreSQL 用户表与初始化管理员；
- bcrypt 用户名密码登录；
- 注册流程固定创建 `user` 角色，成功返回 `201`，重复用户名返回 `409`；
- Casbin RBAC 使用 GORM Adapter 持久化到 PostgreSQL；
- 当前只支持 `user`、`admin`、`test` 三种角色，三者都只有 `dashboard:view`（查看首页）权限；
- 本地 HS256 access JWT，不提供 JWKS；
- Redis refresh token，Redis key 只保存原 token 的 SHA-256；
- refresh token 通过 HttpOnly、SameSite=Lax Cookie 返回；
- refresh 时原子删除旧 token 并创建新 token，旧 token 不能重放；
- logout 删除 Redis token，且没有 Cookie 时仍幂等成功；
- 登录和 refresh 会把 Casbin 计算出的角色、权限写入 access JWT；
- `/users/me` 和 `/auth/verify` 经 JWT Filter 后回查数据库，并返回 Casbin 角色与权限；
- Gateway 只代理唯一单体 API。

当前路由：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/api/v1/auth/login` | 返回 access token，并设置 refresh Cookie |
| `POST` | `/api/v1/auth/register` | 注册本地 `user` 用户，不允许客户端选择角色 |
| `POST` | `/api/v1/auth/refresh` | 轮换 refresh Cookie，返回新 access token |
| `POST` | `/api/v1/auth/logout` | 删除 refresh token 并清除 Cookie |
| `GET` | `/api/v1/users/me` | Bearer JWT 验证并返回当前用户 |
| `GET` | `/api/v1/auth/verify` | 当前用户验证兼容入口 |
| `GET` | `/health/live` | 进程 liveness |
| `GET` | `/health/ready` | API 检查 PostgreSQL 和 Redis；Gateway 代理该结果 |

## Compose 启动

根目录已经放置本地 `.env`，Compose 会自动读取：

```powershell
docker compose up --build -d
docker compose ps -a
```

根目录 [docker-compose.yml](docker-compose.yml) 是当前项目的独立 Compose Stack，名称为 `ai-gateway-go-auth`。启动顺序为 PostgreSQL healthy → `migrate` 执行完成 → API healthy → Gateway；Redis 与 migration 并行准备。`migrate` 正常完成后显示为 `Exited (0)`，这是一次性任务的预期状态。

版本文件位于 `backend/migrations/`，由 `migrate/migrate:v4.19.1` 执行；执行版本记录在 PostgreSQL 的 `schema_migrations` 表。API 不再运行 `AutoMigrate`，数据库结构变更必须新增成对的 `*.up.sql`、`*.down.sql`。当前版本：

```text
000001_users
000002_casbin_rbac
```

PostgreSQL 已运行时可单独应用尚未执行的版本：

```powershell
docker compose run --rm migrate
```

也可以在 Git Bash 或 WSL 中从仓库根目录运行数据库更新脚本；脚本会先启动 PostgreSQL，再应用所有待执行版本：

```bash
bash .scripts/update-database.sh
```

默认只开放 `http://127.0.0.1:8080`。当前 `.env` 使用开发凭据，部署前必须修改管理员密码、PostgreSQL 密码和 `JWT_SECRET`。

完整登录、刷新和注销示例：

```powershell
$session = New-Object Microsoft.PowerShell.Commands.WebRequestSession

$login = Invoke-RestMethod `
  -Method Post `
  -Uri 'http://127.0.0.1:8080/api/v1/auth/login' `
  -ContentType 'application/json' `
  -Body '{"username":"admin","password":"admin12345"}' `
  -WebSession $session

Invoke-RestMethod `
  -Uri 'http://127.0.0.1:8080/api/v1/users/me' `
  -Headers @{ Authorization = "Bearer $($login.data.accessToken)" }

$refreshed = Invoke-RestMethod `
  -Method Post `
  -Uri 'http://127.0.0.1:8080/api/v1/auth/refresh' `
  -WebSession $session

Invoke-RestMethod `
  -Method Post `
  -Uri 'http://127.0.0.1:8080/api/v1/auth/logout' `
  -WebSession $session
```

注册请求沿用原后端合同；当前只检查 `code` 非空，尚未接入验证码发送或校验服务：

```powershell
Invoke-RestMethod `
  -Method Post `
  -Uri 'http://127.0.0.1:8080/api/v1/auth/register' `
  -ContentType 'application/json' `
  -Body '{"username":"new-user","password":"password123","check_password":"password123","code":"123456","iAgree":true}'
```

## 前端开发

Vue 3 管理界面位于与 `backend/` 同级的 `front/`。开发服务器会把 `/api` 和 `/health` 请求代理到本机 `127.0.0.1:8080`，因此先启动后端，再启动前端：

```powershell
cd front
npm install
npm run dev
```

已安装前端依赖后，也可以在 Git Bash 或 WSL 中从仓库根目录一并启动前后端。脚本会先执行 `docker compose up -d --build` 启动后端，再以前台进程启动 Vite；按 `Ctrl+C` 只停止前端，后端容器继续运行。脚本兼容在 WSL 中复用 Windows Node.js 和 `node_modules`：

```bash
bash .scripts/start-frontend.sh
```

浏览器访问 `http://127.0.0.1:5173/login`。登录页可进入 `/register` 创建账号；注册成功后会返回登录页、回填新用户名并显示成功提示，但不会自动登录。登录成功后自动进入首页；未登录访问首页会返回登录页。生产构建和前端测试：

```powershell
npm run build
npm test
```

生产构建使用同源 `/api/v1` 与 `/health` 路径，部署时应让静态页面和 Gateway 使用同一站点来源，或由外层反向代理统一转发这些路径。

## 配置

`.env` 和 `.env.example` 位于仓库最外层。重要变量：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `JWT_SECRET` | 无默认值 | 根 `.env` 中必填的 HS256 随机密钥，至少 32 字符 |
| `JWT_ACCESS_TTL` | `15m` | access token 有效期 |
| `JWT_REFRESH_TTL` | `168h` | refresh token 与 Cookie 有效期 |
| `COOKIE_SECURE` | `false` | HTTPS 部署时必须设为 `true` |
| `REDIS_ADDR` | `localhost:6379` | 本地直接运行时的 Redis 地址 |
| `REDIS_DB` | `0` | refresh token Redis DB |
| `BOOTSTRAP_ADMIN_USERNAME` | `admin` | 首次启动创建的管理员 |
| `BOOTSTRAP_ADMIN_ROLE` | `admin` | 初始化用户角色，只允许 `user`、`admin`、`test` |

Compose 内部会将 API 的数据库和 Redis 地址覆盖为 `postgres:5432` 与 `redis:6379`。

## 后端开发

Go module 位于 `backend/`：

```powershell
go -C backend test ./...
go -C backend vet ./...
go -C backend build ./cmd/...
```

直接运行 API 需要本机已有 PostgreSQL、Redis，目标数据库已应用 `backend/migrations`，并把根 `.env` 的变量导入当前进程：

```powershell
go -C backend run ./cmd/api
```

架构边界见 [ARCHITECTURE.md](ARCHITECTURE.md)，文件索引见 [CODEBASE.md](CODEBASE.md)。
