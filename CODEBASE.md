# AI Gateway Go V2 Codebase

本文索引当前全部主代码类型和测试。完整 Go module 位于 `backend/`。

## 1. 根目录

| 路径 | 职责 |
| --- | --- |
| `AGENTS.md` | 面向所有贡献者的仓库级代理规则，约束项目文档同步和提交前校验 |
| `.env` | 本地 Compose 配置，Git 忽略，位于仓库最外层，包含容器内 `LOG_FILE` 路径 |
| `.env.example` | 可提交的根配置模板，只包含演示占位值，不保存本地日志路径或真实凭据 |
| `.scripts/` | 本地 Bash 维护脚本：启动 Compose 后端与 Vite 前端、启动 PostgreSQL 并应用待执行 migration |
| `backend/` | Go module、源码、测试、后端 Dockerfile 和本地 Compose Stack |
| `frontend/` | Vue 3 + Vite 管理界面，以及构建静态产物并由 Nginx 提供服务的生产镜像 |
| `docs/deployment.md` | 公开源码仓库、GHCR 四镜像和私有 `erent-deploy` 的发布/部署边界 |

旧 `deploy/`、K8s 和移动后空目录已经删除。

`.scripts/start.sh` 检查 Docker、Node.js、npm 与前端依赖，显式传入根 `.env` 和 `backend/docker-compose.yml`，构建并启动 PostgreSQL、migration、Redis、API、Gateway、Nginx Web 后启动 `frontend/` 的 Vite 开发服务器，并兼容在 WSL 中复用 Windows Node.js；`.scripts/update-database.sh` 使用同一 Compose 入口启动 PostgreSQL，并通过 `migrate` job 应用所有待执行版本。

## 2. `frontend`

### 工程入口与路由

- `package.json`、`package-lock.json`：Vue 3、Vue Router、Axios、Lucide Vue 与 Vite 依赖，以及 `dev`、`build`、`test`、`preview` 脚本；
- `vite.config.js`：Vue 插件及开发期 `/api`、`/health` 到 `127.0.0.1:8080` 的代理；
- `Dockerfile`、`.dockerignore`：Node.js stage 以锁文件安装依赖并生成 `dist`，Nginx stage 只携带生产静态产物和运行配置；
- `nginx.conf`：在容器 `8080` 提供 SPA fallback 和缓存策略，把 `/api/`、`/health/`、`/oai/` 代理到 Compose `gateway:8080`，API 代理关闭缓冲并保留客户端转发头；
- `index.html`、`src/main.js`、`src/App.vue`：单页应用 HTML、Vue 挂载和根路由出口；
- `src/router/index.js`：`/login`、`/register` 与受保护的 `/` 路由，基于 access token 执行访客/登录态跳转。

### 页面、组件与认证

- `src/views/LoginView.vue`：用户名密码表单、注册入口、注册成功提示与用户名回填、记住登录状态、密码显隐、错误反馈和登录后重定向；
- `src/views/RegisterView.vue`：用户名、注册码、密码确认和使用确认表单；注册成功后返回登录页，不自动建立会话；
- `src/views/DashboardView.vue`：当前用户、角色、权限、readiness、能力边界、响应式侧栏、主题切换和退出登录；
- `src/components/AppLogo.vue`：登录页与侧栏共用的 AI Gateway 标识；
- `src/axios/config.js`：Axios 默认超时，以及注入 Bearer access token、解包响应数据的默认拦截器；
- `src/axios/service.js`：业务请求与 refresh 专用 Axios 实例、Cookie 传输，以及并发共享的单次 401 refresh/retry；
- `src/axios/index.js`：面向业务 API 的统一 `get`、`post`、`put`、`delete` 请求入口；
- `src/services/session.js`：access token 的本地/会话存储、用户名记忆、刷新后 token 替换和清理；
- `src/services/auth.js`：通过 Axios 统一入口实现注册、登录、logout、当前用户、readiness 与业务错误中文映射；
- `src/styles/main.css`：参考 CLI Proxy API Management Center 的暖白/黑色主题 tokens、全局基础样式与深色主题；
- `tests/auth.test.js`：覆盖持久/会话 token 保存、登录 401、注册请求合同、重复用户名中文反馈、会话清理、401 刷新重试与并发刷新合并。

## 3. `backend/cmd`

### `api/`

`main.go` 只保留进程退出码、资源生命周期和 HTTP 启动编排；`config.go` 聚合通用运行配置与 OAI OIDC 配置加载；`instances.go` 初始化 logger、PostgreSQL、Redis、Repository、Casbin、JWT 与可选 OIDC、upstream gRPC 连接和 directory 实例，并统一关闭外部资源；`bootstrap.go` 校验初始化角色并幂等创建初始用户；`health.go` 注册 liveness/readiness，其中 readiness 检查 PostgreSQL、Redis，OAuth 启用时额外检查 upstream gRPC health；`routes.go` 创建 `gin.Engine`、挂载通用 Middleware、`/ping`、业务路由和可选 OAI OAuth 路由。生产数据库 migration 仍完全由 Compose migration job 执行，API 不执行自动迁移、不创建 Handler，也不使用 `http.Server`。

`bootstrap_test.go` 覆盖空配置、非法角色和幂等创建；`health_test.go` 覆盖健康端点及 Redis 不可用时的 readiness `503`。

### `upstream/main.go`、`grpc-healthcheck/main.go`

`upstream` 加载独立的 gRPC/超时/mTLS 与 OAI 配置，初始化 provider discovery，同步阻塞运行 gRPC 服务，收到 SIGTERM 后排空请求，超时强制关闭。`grpc-healthcheck` 在 2s 内调用标准 gRPC health，可使用单独的客户端证书；进程不依赖 JWT、数据库或 Redis。

### `gateway/main.go`、`main_test.go`

可选单上游反向代理；`gatewayConfig`、`loadGatewayConfig`、`newProxy`、`newGatewayRouter` 分别负责配置、代理和 Gin 路由。`main` 以 `router.Run` 启动。测试覆盖本地 `/health/live`、`/ping`、转发与非法地址。

### `healthcheck/main.go`

distroless 容器 readiness 客户端，默认访问 `127.0.0.1:8080/health/ready`。Dockerfile 将它复制到 API/Gateway 镜像的 `/healthcheck`，Compose 直接执行该客户端；API `main` 暴露被探测端点，二者不能互相替代。

## 4. `backend/internal/config`

### `config.go`

- `Config`：HTTP 地址、PostgreSQL、OIDC discovery 超时、Cookie 和初始化管理员，并通过嵌套的 `RedisConfig`、`JWTConfig` 聚合 Redis 连接参数及必填 JWT Secret、issuer、audience、access/refresh TTL；
- `OIDCConfig`、`LoadOIDCConfig`：按 provider 名生成环境变量前缀，加载 issuer、client、secret 与 redirect URL；整组为空时禁用，部分配置或非法 URL 会拒绝启动；
- `Load`/`load`：环境变量加载与校验，JWT Secret 没有代码默认值且至少 32 字符；
- `positiveDuration`、`nonNegativeInt`、`booleanValue`、`validateAddress`：解析辅助函数。

`upstream.go` 定义 `UpstreamClientConfig`、`UpstreamServerConfig` 和 `GRPCTLSConfig`，分别加载目标/监听地址、RPC/token/discovery/shutdown 超时与成组的 mTLS 证书路径；`upstream_test.go` 覆盖默认值、覆盖值及非法地址、时长和证书组合。

### `config_test.go`

覆盖默认值、覆盖值、Redis DB、Cookie bool、JWT Secret/TTL、OIDC discovery 超时、provider 专属 OIDC 配置、非法配置和管理员组合。

## 5. `backend/internal/logger`

### `logger.go`、`logger_test.go`

- `NewLogger`：预创建并验证 `LOG_FILE` 指定的文件，返回同时写标准输出和 lumberjack 轮转文件的 JSON `slog.Logger` 以及待关闭资源；空路径默认使用 `./logs/backend/app.log`；
- `dualWriter`：文件 sink 运行期失败时写入标准错误，同时保持控制台日志可用；
- 轮转边界：单文件 100 MB、10 个备份、保留 30 天并压缩；
- 测试覆盖嵌套目录创建、控制台/文件 JSON 双写、不可用目录拒绝和运行期文件错误报告。

## 6. 数据与 DTO

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

## 7. HTTP/JWT/Redis/Casbin Middleware

### `middleware/httpserver/middleware.go`

`RequestID`、`Recovery`、`AccessLog` 处理通用传输行为；`IsValidRequestID` 供测试使用。

### `middleware/jwt`

- `claims.go`：`Claims` 和 `ErrInvalidToken`；
- `manager.go`：`JWTManager`、接收 `config.JWTConfig` 的 `NewJWTManager`、`GenerateToken`、`GenerateTokenWithPermissions`、`ParseToken`，并保存 `RefreshTTL`；
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

- `connect.go`：`Connect` 接收 `config.RedisConfig`，创建并验证 Redis client；
- `connect_test.go`：使用隔离 Redis 验证 `RedisConfig` 的地址和 DB 会传入 client；
- `oauth_state.go`：以 `oidc:flow:<state>` 保存 OAuth 登录流程，并通过 Redis `GETDEL` 原子读取删除；
- `refresh_token.go`：`CreateRefreshToken`、`RotateRefreshToken`、`DeleteRefreshToken`；
- `newRefreshToken`：生成 256-bit token；
- `refreshTokenKey`：SHA-256 Redis key；
- `rotateRefreshTokenScript`：旧 token 删除和新 token 建立的 Lua 原子操作；
- `refresh_token_test.go`：随机长度、key 隐私、创建/轮换/重放拒绝/删除。

## 8. `modules/auth`

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

## 9. `modules/oauth`

### 通用 OAuth 层

- `oauth_handler.go`：`OauthHandler`/`NewOauthHandler`、`Login`、`Callback`；生成随机 state 与 PKCE verifier，映射无效 state、provider 拒绝、兑换及保存错误；Handler 直接依赖统一的具体 `*OauthService`；
- `oauth_service.go`：`OauthService`/`NewOauthService`、`StoreFlow`、`PopFlow`、`AuthCodeURL`、`Exchange`、`SaveToken` 与 `ErrInvalidOAuthState`；使用 Redis 一次性消费登录流程并根据注入的 `OIDCAuth` 生成授权地址，通过使用方定义的 `TokenExchanger` 接口调用 directory 并传递固定 provider；`SaveToken` 当前为空实现；
- `errors.go`：定义不依赖 gRPC 的兑换业务错误；`oauth_exchange_test.go` 验证参数传递、HTTP 400/502/503/504 映射和错误信息隔离。
- `oauth_routes.go`：注册 `GET /login` 与 `GET /callback`，实际前缀由调用方的 Gin group 决定；
- `oauth_handler_test.go`：使用真实 Service 和 miniredis 覆盖缺失、无效、过期 state 及 Redis 故障的 HTTP 映射；
- `oauth_service_test.go`：覆盖 provider 授权参数、S256 PKCE、state 一次性消费、Redis 故障及损坏状态数据。

### `oidc/oidc.go`

`LoginFlow` 保存 PKCE verifier 和流程过期时间；`OIDCAuth` 聚合 discovery 后的 `oauth2.Config` 与 provider 专属授权 URL 参数；`NewOIDCAuth` 通过 issuer discovery 构造 authorization/token endpoint，并明确选择 provider 支持的认证方式，避免兑换时探测方式引起重复请求。Redis key 直接使用高熵 state，不额外保存 provider ID。

### `openai/oai_config.go`

提供 OpenAI 的 `prompt=login`、`id_token_add_organizations=true`、`codex_cli_simplified_flow=true` 授权参数。`OaiScopes` 当前返回空集合，token 持久化、完成跳转、nonce 和 ID token 验证也尚未实现，因此该模块仍是登录流程骨架。

## 10. `modules/user`

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

## 11. `internal/router`

`routerall.go` 的 `AuthRouter`、`UserRouter` 接收具体的 `*user.Repository` 和 Casbin Enforcer，在路由包内创建 Service/Handler 并调用模块 `Register*Routes`；`OauthRouter` 接收 Redis client、`*oidc.OIDCAuth`、`oauth.TokenExchanger` 与 provider，创建统一的 OAuth Service/Handler，因此可由不同 Gin group 和 OIDC 配置复用。健康检查和 `/ping` 由 `cmd/api` 注册。`routerall_test.go` 使用内存 SQLite 的真实 GORM Repository，覆盖注册、重复用户名、注册后登录、Casbin 首页权限、refresh Cookie、rotation、logout、当前用户、健康、Request ID 和未认证拒绝。

## 12. `internal/testdatabase`

`database.go` 的 `Open` 为测试创建相互隔离且开启 `TranslateError` 的纯 Go 内存 SQLite 数据库。测试自行对 SQLite 执行 `AutoMigrate`，用于验证真实 GORM Repository 调用链；它不进入生产启动路径，也不替代 PostgreSQL SQL migration。

## 13. upstream 远程适配

- `proto/upstream.proto`：ExchangeCode、预留 RefreshToken 和 token 响应；使用 Timestamp 表达可选有效期。
- `internal/rpc/upstream/*.pb.go`：由 protoc 生成的消息、客户端和服务端注册代码，不手工编辑。
- `internal/directory/upstream/upstreamdirectory.go`：实现 OAuth TokenExchanger，设置 RPC deadline，转换请求/响应和错误；不重试授权码。
- `internal/upstreamserver/server.go`：校验请求/provider，使用 PKCE VerifierOption 兑换，映射 provider 错误，注册标准 health，处理有界排空。
- `internal/upstreamserver/server_test.go`：真实 gRPC 编解码配合模拟 OIDC/token 服务，覆盖 PKCE、token 字段、回调链路、错误、deadline、单次兑换与停止行为。
- `internal/rpc/transport/tls.go`、`tls_test.go`：加载 mTLS 身份与 CA；验证合法连接、缺少客户端身份、错误服务器名及不受信任 CA。
- `.scripts/update-grpc.ps1`：从协议源重新生成 Go 文件；具体工具版本与启动方式见 `docs/upstream.md`。

## 14. 构建与维护

| 文件 | 职责 |
| --- | --- |
| `backend/Dockerfile` | 构建 API、Gateway、Upstream 三个 distroless target 及 HTTP/gRPC healthcheck 工具，并提供携带 SQL 的 `migrations` 发布 target；API 镜像为 UID 65532 创建日志目录 |
| `frontend/Dockerfile`、`nginx.conf` | 构建 Vue `dist`，再生成提供 SPA 静态文件并同源代理 Gateway 的 Nginx Web 镜像 |
| `backend/docker-compose.yml` | `ai-gateway-go-auth` 本地 Stack：PostgreSQL、Redis、一次性 migrate job、API、Upstream、Gateway、Web；后端路径相对于 `backend/`，Web context 指向 `../frontend` |
| `backend/migrations/` | PostgreSQL schema 的版本化 up/down SQL |
| `backend/.dockerignore` | 限定后端构建上下文 |
| `backend/go.mod`、`go.sum` | Gin、GORM、Casbin GORM Adapter、JWT、Redis、OIDC、OAuth2、gRPC、Protobuf、lumberjack，以及仅用于测试的纯 Go SQLite 依赖 |
| `.github/workflows/ci.yml` | 分别校验 Go 与 Vue，构建 `backend/` 的 API/Gateway/Upstream/Migrations targets 和 `frontend/` 的 Web 镜像，全部只验证、不推送 |
| `.github/workflows/release.yml` | `main`、`v*` tag 或手动触发时向 GHCR 发布 API、Gateway、Upstream、Migrations、Web 五个同源码版本镜像，附加完整提交 SHA、default-branch `latest` 和 tag 标签 |

维护规则：Handler 不直接访问 GORM；Service 直接依赖具体 Repository；Handler 需要替换 Service 时由 Handler 定义最小接口；Repository 不处理 HTTP/Cookie；refresh token 原值不进入日志或 Redis key；改变路由、模型、配置、测试或目录时同步本文和 `ARCHITECTURE.md`。
