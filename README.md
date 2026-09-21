# erent 本地部署

AI Gateway 提供本地登录、OAuth 账号管理、API Key 管理及聊天请求转发。本文从首次克隆开始说明本地 Docker 部署；项目设计、OAuth 与 gRPC 边界见 [项目架构](docs/README.md)。

## 1. 准备环境

- Git、Docker Engine / Docker Desktop，以及 Docker Compose v2。
- 能拉取基础镜像和构建依赖的网络；启用 OAuth 时 upstream 还需访问提供方。
- 默认使用本机端口 5432、8080、8088、1455，请先确保未被其他服务占用。
- 全容器部署不需要本机安装 Go 或 Node.js。

```powershell
git clone https://github.com/DWHuang99/erent.git
cd erent
Copy-Item .env.example .env
```

已有 `.env` 时保留现有配置，不要覆盖。以下命令均从仓库根目录执行。

## 2. 配置环境变量

编辑 `.env`：

| 变量 | 本地配置说明 |
| --- | --- |
| `POSTGRES_DB` / `POSTGRES_USER` / `POSTGRES_PASSWORD` | 数据库名称、用户和密码；替换示例密码 |
| `JWT_SECRET` | 替换为至少 32 字符的随机密钥 |
| `BOOTSTRAP_ADMIN_USERNAME` / `BOOTSTRAP_ADMIN_PASSWORD` | 首次初始化管理员的登录凭据；替换示例值 |
| `APP_PORT` / `WEB_PORT` | Gateway / Web 宿主机端口，默认 8080 / 8088 |
| `COOKIE_SECURE` | 本地 HTTP 使用 `false`；HTTPS 部署使用 `true` |
| `OAI_ISSUER` / `OAI_CLIENT_ID` / `OAI_REDIRECT_URL` | 可选 OAuth 配置；整组留空可先运行本地登录与管理页面 |
| `OAI_CLIENT_SECRET` | 按提供方要求配置，公开客户端可留空 |
| `OAUTH_ENCRYPTION_KEY` | 启用 OAuth 时必填，Base64 编码的随机 32 字节 AES 密钥；持久保管，更换后旧凭据无法解密 |

PowerShell 可用以下命令生成随机密钥，分别生成 JWT 和 OAuth 密钥：

```powershell
$randomBytes = New-Object byte[] 32
$rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
$rng.GetBytes($randomBytes)
[Convert]::ToBase64String($randomBytes)
$rng.Dispose()
```

Compose 自动把容器内数据库、Redis 和 gRPC 地址设置为服务名；直接运行 Go 时才使用 `.env.example` 中的本机地址，并确保 `DATABASE_URL` 与数据库凭据一致。Go 进程本身不加载 `.env`。

## 3. 构建并启动

```powershell
docker compose --env-file .env -f backend/docker-compose.yml config --quiet
docker compose --env-file .env -f backend/docker-compose.yml up --build -d
docker compose --env-file .env -f backend/docker-compose.yml ps -a
```

Stack 包括 PostgreSQL、Redis、一次性 migrate、API、upstream、Gateway、Web 和本地 OAuth 回调接收服务。API 等待数据库、Redis、upstream 健康以及迁移成功后启动。`migrate` 显示 `Exited (0)` 属于正常状态。

- 控制台：[http://127.0.0.1:8088/login](http://127.0.0.1:8088/login)，使用 `.env` 中配置的管理员账号登录。
- Gateway：[http://127.0.0.1:8080](http://127.0.0.1:8080)，用于直接调用 API。
- 健康检查：

```powershell
Invoke-RestMethod http://127.0.0.1:8080/health/ready
docker compose --env-file .env -f backend/docker-compose.yml logs --tail 100 api upstream gateway web
```

初始化管理员采用幂等创建；修改 `.env` 的初始化密码不会重置已存在用户的密码。PostgreSQL 已初始化数据卷时，更改环境变量也不会自动修改数据库中的用户密码。

## 4. 授权账号与 API Key

启用 OAuth 后，在控制台进入授权页面，可选择设备授权或浏览器授权。浏览器方式使用提供方允许的回调地址；本地 Codex 回调入口为 `http://localhost:1455/auth/callback`。Compose 中的接收服务会将查询参数转交本地 Web 控制台。API 与 upstream 的 provider 配置必须一致，详见 [项目架构中的授权流程](docs/README.md#授权流程)。

授权保存成功后，在“API Key”页面选择自己的已授权账号并创建密钥。完整密钥仅在创建时返回，请即时保存；列表仅显示前缀。页面支持修改有效期、启用/禁用、替换关联账号和删除。

聊天接口使用 `Authorization: Bearer <API_KEY>`，通过 **Gateway 的 8080 端口**调用：

| 方法 | 当前实际路径 | 请求格式 |
| --- | --- | --- |
| POST | `/v1/chat/completions` | OpenAI Chat Completions |
| POST | `/v1/responses` | OpenAI Responses |
| POST | `/v1/messages` | Claude Messages |

当前 Web Nginx 通过 `/v1/` 代理上述聊天路径。客户端需要支持配置实际请求 URL。模型与凭据组合以 `backend/internal/modules/chat/route/init.go` 中注册的路由为准。当前只选取首个符合条件的关联账号，无自动轮换或失败切换；过期凭据需在账号页面手动刷新。

## 5. 停止、更新与数据

```powershell
# 停止服务并保留数据卷
docker compose --env-file .env -f backend/docker-compose.yml down

# 更新代码后重新构建并应用待执行迁移
git pull --ff-only
docker compose --env-file .env -f backend/docker-compose.yml up --build -d

# 单独执行数据库迁移（PostgreSQL 已启动）
docker compose --env-file .env -f backend/docker-compose.yml run --rm migrate
```

PostgreSQL、Redis、API 日志和 upstream 日志分别存放在 named volumes。需要保留数据时不要使用 `down -v`，该参数会删除数据卷。升级前备份数据库及 OAuth 加密密钥。迁移版本位于 `backend/migrations/`，当前包含 000001–000005（用户、RBAC、OAuth 凭据、账号归属、API Key）。生产进程不自动修改表结构。

## 6. 可选：本地开发

前端使用 Node.js 22，后端 Go 版本以 `backend/go.mod` 为准。

```powershell
cd frontend
npm ci
npm run dev
```

Vite 默认入口为 [http://127.0.0.1:5173](http://127.0.0.1:5173)，请求代理到本机 8080。Git Bash / WSL 可在根目录运行 `bash .scripts/start.sh` 同时启动容器与 Vite；`bash .scripts/start.sh debug` 启动 PostgreSQL、Redis、迁移和回调服务，再由 IDE 启动 API 与 upstream。Ctrl+C 只停止 Vite，容器继续运行。

```powershell
go -C backend test ./...
go -C backend vet ./...
go -C backend build ./cmd/...
npm --prefix frontend test
npm --prefix frontend run build
```

## 常见排查

- `JWT_SECRET must be set`：确认已创建根 `.env`，且命令包含 `--env-file .env`。
- API 未就绪：查看 `migrate`、PostgreSQL、Redis 和 upstream 日志；启用 OAuth 后检查 provider 配置与网络。
- OAuth 回调失败：检查 1455 端口，以及 `OAUTH_CONSOLE_ORIGIN` 是否指向发起授权的同一控制台；修改配置后重新创建容器并重新授权。
- 设备授权取消或超时：先检查已授权账号是否保存成功，再重新申请设备码。
- 聊天返回 401 / 403 / 503：分别检查密钥状态、模型关联账号以及账号凭据有效期和 upstream 可用性。
