# AI Gateway Go V2 Codebase

本文索引当前全部主代码类型和测试。完整 Go module 位于 `backend/`。

## 1. 根目录

| 路径 | 职责 |
| --- | --- |
| `.env` | 本地 Compose 配置，Git 忽略，位于仓库最外层 |
| `.env.example` | 可提交的根配置模板 |
| `.scripts/` | 本地 Bash 维护脚本：启动 Compose 后端与 Vite 前端、启动 PostgreSQL 并应用待执行 migration |
| `docker-compose.yml` | `ai-gateway-go-auth` Stack：PostgreSQL、Redis、一次性 migrate job、单体 API、Gateway |
| `backend/` | Go module、源码、测试和 Dockerfile |
| `front/` | Vue 3 + Vite 管理界面，包含登录、受保护首页和认证 API 适配 |

旧 `deploy/`、K8s 和移动后空目录已经删除。

`.scripts/start-frontend.sh` 检查 Docker、Node.js、npm 与前端依赖，通过根 Compose 构建并启动 PostgreSQL、migration、Redis、API、Gateway 后启动 `front/` 的 Vite 开发服务器，并兼容在 WSL 中复用 Windows Node.js；`.scripts/update-database.sh` 检查 Docker 可用性，启动 PostgreSQL 后通过 Compose `migrate` job 应用所有待执行版本。

## 2. `front`

### 工程入口与路由

- `package.json`、`package-lock.json`：Vue 3、Vue Router、Lucide Vue 与 Vite 依赖，以及 `dev`、`build`、`test`、`preview` 脚本；
- `vite.config.js`：Vue 插件及开发期 `/api`、`/health` 到 `127.0.0.1:8080` 的代理；
- `index.html`、`src/main.js`、`src/App.vue`：单页应用 HTML、Vue 挂载和根路由出口；
- `src/router/index.js`：`/login`、`/register` 与受保护的 `/` 路由，基于 access token 执行访客/登录态跳转。

### 页面、组件与认证

- `src/views/LoginView.vue`：用户名密码表单、注册入口、注册成功提示与用户名回填、记住登录状态、密码显隐、错误反馈和登录后重定向；
- `src/views/RegisterView.vue`：用户名、注册码、密码确认和使用确认表单；注册成功后返回登录页，不自动建立会话；
- `src/views/DashboardView.vue`：当前用户、角色、权限、readiness、能力边界、响应式侧栏、主题切换和退出登录；
- `src/components/AppLogo.vue`：登录页与侧栏共用的 AI Gateway 标识；
- `src/services/auth.js`：注册、登录、logout、当前用户、access token 存储、Bearer 请求以及一次 refresh/retry；
- `src/styles/main.css`：参考 CLI Proxy API Management Center 的暖白/黑色主题 tokens、全局基础样式与深色主题；
- `tests/auth.test.js`：覆盖持久/会话 token 保存、登录 401、注册请求合同、重复用户名中文反馈和会话清理。

## 3. `backend/cmd`

### `api/main.go`

加载配置并连接已由 Compose migration job 初始化的 PostgreSQL 和 Redis，创建 User Repository、Casbin Enforcer 与 JWT Manager。`main` 不执行生产数据库 migration；它校验初始化角色只能是 `user`、`admin`、`test`，创建 `gin.Engine`、注册 `/ping` 与健康检查后调用 `apirouter.AuthRouter`/`UserRouter`，最后 `router.Run(HTTP_ADDR)`。不创建 Handler，也不使用 `http.Server`。Readiness 同时检查 PostgreSQL 和 Redis。

### `gateway/main.go`、`main_test.go`

可选单上游反向代理；`gatewayConfig`、`loadGatewayConfig`、`newProxy`、`newGatewayRouter` 分别负责配置、代理和 Gin 路由。`main` 以 `router.Run` 启动。测试覆盖本地 `/health/live`、`/ping`、转发与非法地址。

### `healthcheck/main.go`

容器 readiness 客户端，默认访问 `127.0.0.1:8080/health/ready`。

## 4. `backend/internal/config`

### `config.go`

- `Config`：HTTP 地址、PostgreSQL、Redis、必填 JWT Secret、access/refresh TTL、Cookie 和初始化管理员；
- `Load`/`load`：环境变量加载与校验，JWT Secret 没有代码默认值且至少 32 字符；
- `positiveDuration`、`nonNegativeInt`、`booleanValue`、`validateAddress`：解析辅助函数。

### `config_test.go`

覆盖默认值、覆盖值、Redis DB、Cookie bool、JWT Secret/TTL、非法配置和管理员组合。

## 5. 数据与 DTO

### `database/connect/connect.go`

`Connect` 通过 GORM PostgreSQL driver 建立连接、开启 `TranslateError`、配置连接池并执行 `PingContext`。

### `backend/migrations`

- `000001_users.up.sql` / `.down.sql`：创建/删除 `users`，角色字段限制为 `user`、`admin`、`test`；
- `000002_casbin_rbac.up.sql` / `.down.sql`：创建/删除 `casbin_rule`，初始化三角色 `dashboard:view` policy，并迁移已有用户 grouping；
- Compose 的 `migrate` 服务使用 `migrate/migrate:v4.19.1` 执行并由 `schema_migrations` 记录版本；API 不运行 `AutoMigrate`。

### `dto/request/auth.go`

`LoginRequest` 定义登录 JSON 和 Gin 校验；`RegisterRequest` 沿用原前端字段 `username`、`password`、`check_password`、`code`、`iAgree`。

### `dto/response/response.go`、`user.go`

`Response`、`Success`、`SuccessWithStatus`、`Error` 统一 `{code,data,message}`；`UserInfo` 是不含密码的当前用户响应。

## 6. HTTP/JWT/Redis/Casbin Middleware

### `middleware/httpserver/middleware.go`

`RequestID`、`Recovery`、`AccessLog` 处理通用传输行为；`IsValidRequestID` 供测试使用。

### `middleware/jwt`

- `claims.go`：`Claims` 和 `ErrInvalidToken`；
- `manager.go`：`JWTManager`、`NewJWTManager`、`GenerateToken`、`GenerateTokenWithPermissions`、`ParseToken`，并保存 `RefreshTTL`；
- `filter.go`：`JwtFilter` 与身份 Context keys；
- `manager_test.go`：签发、过期、错误密钥；
- `filter_test.go`：合法身份和缺少 Bearer token。

目录没有 JWKS、公钥发现或远程 keyfunc。

### `middleware/casbin`

- `model.conf`：最小 subject-object RBAC 模型；
- `NewEnforcer`：关闭 Adapter AutoMigrate，通过 GORM Adapter 读取和持久化已由 SQL migration 创建的 `casbin_rule`；
- `NewMemoryEnforcer`：测试使用的同模型内存 Enforcer；
- `SeedDefaultPolicies`：为 `user`、`admin`、`test` 写入唯一的 `dashboard:view` 权限；
- `AuthorizationForUser`：把数据库 `role_code` 同步为用户 grouping，并返回有效 roles/permissions；
- `IsSupportedRole`、`UserSubject`、`RoleSubject`、`RoleCodes`、`PermissionCodes`：角色校验与 Casbin 数据转换；
- `casbin_test.go`：覆盖三角色仅可查看首页、持久化 Adapter 和非法角色拒绝。

### `middleware/redis`

- `connect.go`：`Connect` 创建并验证 Redis client；
- `refresh_token.go`：`CreateRefreshToken`、`RotateRefreshToken`、`DeleteRefreshToken`；
- `newRefreshToken`：生成 256-bit token；
- `refreshTokenKey`：SHA-256 Redis key；
- `rotateRefreshTokenScript`：旧 token 删除和新 token 建立的 Lua 原子操作；
- `refresh_token_test.go`：随机长度、key 隐私、创建/轮换/重放拒绝/删除。

## 7. `modules/auth`

### `service.go`

- `AuthService`：直接依赖具体的 `*user.Repository`，并持有 Casbin Enforcer、JWT Manager、Redis client、dummy hash；
- `Login`：密码校验并签发 access/refresh；
- `Register`：校验原注册合同，bcrypt hash 后固定创建 `user`，处理重复用户名并建立 Casbin grouping；
- `Refresh`：Redis rotation、用户回查、状态校验和 access 重签；
- `Logout`：删除 refresh token；
- `RefreshTTL`、`issueTokensForUser`、`accessTokenForUser`：Cookie TTL、Casbin 权限加载和签发辅助；
- `ErrInvalidRequest`、`ErrUserExists`、`ErrInvalidRefreshToken`、`ErrUserDisabled`：业务错误。

### `handler.go`

`AuthHandler` 处理注册、登录、refresh、logout 和 HttpOnly Cookie；`Register` 映射 `201/400/409/500`，`setRefreshCookie`、`clearRefreshCookie` 统一 Cookie 属性。

### `routes.go`

```text
POST /api/v1/auth/login
POST /api/v1/auth/register
POST /api/v1/auth/refresh
POST /api/v1/auth/logout
```

### `service_test.go`

使用内存 SQLite 上的真实 GORM User Repository、Casbin Enforcer 与 miniredis，覆盖注册、固定角色、bcrypt、重复用户名、非法请求、登录、角色/首页权限 claims、轮换、旧 token 重放拒绝、logout、错误密码和禁用用户。

## 8. `modules/user`

### `model.go`

`User` 是 GORM model；`UserAuth` 是认证查询数据；`CurrentUser` 是业务模型。

### `repository.go`

`Repository`/`NewRepository`、`AddUser`、`GetUserAuthByUsername`、`GetUserAuthByID`、`GetUserByID`、`CreateBootstrapUser` 承担全部 GORM 用户访问。Repository 不负责生产 schema migration。

### `service.go`、`service_test.go`

`Service` 直接依赖具体的 `*Repository` 和 Casbin Enforcer；`Service.GetUserByID` 处理不存在、禁用规则并填充有效 roles/permissions。测试通过真实 GORM Repository 覆盖正常、首页权限、不存在和禁用状态。

### `handler.go`、`routes.go`

`CurrentUserService` 是 Handler 使用方定义的最小 Service 接口；`UserHandler.GetCurrentUser` 从 JWT Context 获取 ID，经 Service 回查并输出 `UserInfo`；`ToUserInfoResponse` 完成 DTO 转换。路由：

```text
GET /api/v1/users/me
GET /api/v1/auth/verify
```

## 9. `internal/router`

`routerall.go` 的 `AuthRouter`、`UserRouter` 接收具体的 `*user.Repository` 和 Casbin Enforcer，在路由包内创建 Service/Handler 并调用模块 `Register*Routes`。健康检查和 `/ping` 由 `cmd/api` 注册。`routerall_test.go` 使用内存 SQLite 的真实 GORM Repository，覆盖注册、重复用户名、注册后登录、Casbin 首页权限、refresh Cookie、rotation、logout、当前用户、健康、Request ID 和未认证拒绝。

## 10. `internal/testdatabase`

`database.go` 的 `Open` 为测试创建相互隔离且开启 `TranslateError` 的纯 Go 内存 SQLite 数据库。测试自行对 SQLite 执行 `AutoMigrate`，用于验证真实 GORM Repository 调用链；它不进入生产启动路径，也不替代 PostgreSQL SQL migration。

## 11. 构建与维护

| 文件 | 职责 |
| --- | --- |
| `backend/Dockerfile` | 构建 API、Gateway、healthcheck 两个 distroless target |
| `backend/migrations/` | PostgreSQL schema 的版本化 up/down SQL |
| `backend/.dockerignore` | 限定后端构建上下文 |
| `backend/go.mod`、`go.sum` | Gin、GORM、Casbin GORM Adapter、JWT、Redis，以及仅用于测试的纯 Go SQLite 依赖 |
| `.github/workflows/ci.yml` | 以 `backend/` 为 Go working directory，构建 `./backend` Docker context |

维护规则：Handler 不直接访问 GORM；Service 直接依赖具体 Repository；Handler 需要替换 Service 时由 Handler 定义最小接口；Repository 不处理 HTTP/Cookie；refresh token 原值不进入日志或 Redis key；改变路由、模型、配置、测试或目录时同步本文和 `ARCHITECTURE.md`。
