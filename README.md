# AI Gateway Go V2

Go 后端现在位于独立的 `backend/`，采用与 `D:/vue-element-plus-admin/backend` 相同方向的模块化单体分层：DTO、Middleware、Auth/User Handler-Service-Repository、总 Router 和进程装配彼此分离。数据库访问使用 GORM；多个业务服务、gRPC、JWKS、OIDC 和 OAuth 均未引入。

## 仓库布局

```text
ai-gateway/
├─ backend/              # 完整 Go module 和 Dockerfile
│  ├─ cmd/
│  ├─ internal/
│  ├─ go.mod
│  └─ go.sum
├─ front/
├─ docs/
├─ .env                  # 本地 Compose 配置，位于最外层且不提交
├─ .env.example          # 可提交的配置示例
└─ docker-compose.yml    # Gateway、API、PostgreSQL、Redis
```

仓库不再包含 K8s 或旧部署目录。

## 当前认证能力

- GORM + PostgreSQL 用户表与初始化管理员；
- bcrypt 用户名密码登录；
- 本地 HS256 access JWT，不提供 JWKS；
- Redis refresh token，Redis key 只保存原 token 的 SHA-256；
- refresh token 通过 HttpOnly、SameSite=Lax Cookie 返回；
- refresh 时原子删除旧 token 并创建新 token，旧 token 不能重放；
- logout 删除 Redis token，且没有 Cookie 时仍幂等成功；
- `/users/me` 和 `/auth/verify` 经 JWT Filter 后回查数据库；
- Gateway 只代理唯一单体 API。

当前路由：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/api/v1/auth/login` | 返回 access token，并设置 refresh Cookie |
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
docker compose ps
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

## 配置

`.env` 和 `.env.example` 位于仓库最外层。重要变量：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `JWT_ACCESS_TTL` | `15m` | access token 有效期 |
| `JWT_REFRESH_TTL` | `168h` | refresh token 与 Cookie 有效期 |
| `COOKIE_SECURE` | `false` | HTTPS 部署时必须设为 `true` |
| `REDIS_ADDR` | `localhost:6379` | 本地直接运行时的 Redis 地址 |
| `REDIS_DB` | `0` | refresh token Redis DB |
| `BOOTSTRAP_ADMIN_USERNAME` | `admin` | 首次启动创建的管理员 |

Compose 内部会将 API 的数据库和 Redis 地址覆盖为 `postgres:5432` 与 `redis:6379`。

## 后端开发

Go module 位于 `backend/`：

```powershell
go -C backend test ./...
go -C backend vet ./...
go -C backend build ./cmd/...
```

直接运行 API 需要本机已有 PostgreSQL、Redis，并把根 `.env` 的变量导入当前进程：

```powershell
go -C backend run ./cmd/api
```

架构边界见 [ARCHITECTURE.md](ARCHITECTURE.md)，文件索引见 [CODEBASE.md](CODEBASE.md)，后续范围见 [docs/ROADMAP.md](docs/ROADMAP.md)。
