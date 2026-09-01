# AI Gateway Go V2 Codebase

本文索引当前全部主代码类型和测试。完整 Go module 位于 `backend/`。

## 1. 根目录

| 路径 | 职责 |
| --- | --- |
| `.env` | 本地 Compose 配置，Git 忽略，位于仓库最外层 |
| `.env.example` | 可提交的根配置模板 |
| `docker-compose.yml` | PostgreSQL、Redis、单体 API、Gateway |
| `backend/` | Go module、源码、测试和 Dockerfile |
| `front/` | 保留的前端原型 |
| `docs/ROADMAP.md` | 后续能力范围 |

旧 `deploy/`、K8s 和移动后空目录已经删除。

## 2. `backend/cmd`

### `api/main.go`

加载配置，连接 PostgreSQL 和 Redis，执行 GORM migration/初始化用户，创建 JWT Manager。`main` 创建 `gin.Engine`、注册 `/ping` 与健康检查后调用 `apirouter.AuthRouter`/`UserRouter`，最后 `router.Run(HTTP_ADDR)`。不创建 Handler，也不使用 `http.Server`。Readiness 同时检查 PostgreSQL 和 Redis。

### `gateway/main.go`、`main_test.go`

可选单上游反向代理；`gatewayConfig`、`loadGatewayConfig`、`newProxy`、`newGatewayRouter` 分别负责配置、代理和 Gin 路由。`main` 以 `router.Run` 启动。测试覆盖本地 `/health/live`、`/ping`、转发与非法地址。

### `healthcheck/main.go`

容器 readiness 客户端，默认访问 `127.0.0.1:8080/health/ready`。

## 3. `backend/internal/config`

### `config.go`

- `Config`：HTTP 地址、PostgreSQL、Redis、JWT access/refresh TTL、Cookie 和初始化管理员；
- `Load`/`load`：环境变量加载与校验；
- `positiveDuration`、`nonNegativeInt`、`booleanValue`、`validateAddress`：解析辅助函数。

### `config_test.go`

覆盖默认值、覆盖值、Redis DB、Cookie bool、JWT TTL、非法配置和管理员组合。

## 4. 数据与 DTO

### `database/connect/connect.go`

`Connect` 通过 GORM PostgreSQL driver 建立连接、配置连接池并执行 `PingContext`。

### `dto/request/auth.go`

`LoginRequest` 定义登录 JSON 和 Gin 校验。

### `dto/response/response.go`、`user.go`

`Response`、`Success`、`Error` 统一 `{code,data,message}`；`UserInfo` 是不含密码的当前用户响应。

## 5. HTTP/JWT/Redis Middleware

### `middleware/httpserver/middleware.go`

`RequestID`、`Recovery`、`AccessLog` 处理通用传输行为；`IsValidRequestID` 供测试使用。

### `middleware/jwt`

- `claims.go`：`Claims` 和 `ErrInvalidToken`；
- `manager.go`：`JWTManager`、`NewJWTManager`、`GenerateToken`、`ParseToken`，并保存 `RefreshTTL`；
- `filter.go`：`JwtFilter` 与身份 Context keys；
- `manager_test.go`：签发、过期、错误密钥；
- `filter_test.go`：合法身份和缺少 Bearer token。

目录没有 JWKS、公钥发现或远程 keyfunc。

### `middleware/redis`

- `connect.go`：`Connect` 创建并验证 Redis client；
- `refresh_token.go`：`CreateRefreshToken`、`RotateRefreshToken`、`DeleteRefreshToken`；
- `newRefreshToken`：生成 256-bit token；
- `refreshTokenKey`：SHA-256 Redis key；
- `rotateRefreshTokenScript`：旧 token 删除和新 token 建立的 Lua 原子操作；
- `refresh_token_test.go`：随机长度、key 隐私、创建/轮换/重放拒绝/删除。

## 6. `modules/auth`

### `service.go`

- `UserAuthRepository`：`GetUserAuthByUsername`、`GetUserAuthByID`；
- `AuthService`：用户 Repository、JWT Manager、Redis client、dummy hash；
- `Login`：密码校验并签发 access/refresh；
- `Refresh`：Redis rotation、用户回查、状态校验和 access 重签；
- `Logout`：删除 refresh token；
- `RefreshTTL`、`issueTokensForUser`：Cookie TTL 和签发辅助；
- `ErrInvalidRefreshToken`、`ErrUserDisabled`：业务错误。

### `handler.go`

`AuthHandler` 处理登录、refresh、logout 和 HttpOnly Cookie；`setRefreshCookie`、`clearRefreshCookie` 统一 Cookie 属性。

### `routes.go`

```text
POST /api/v1/auth/login
POST /api/v1/auth/refresh
POST /api/v1/auth/logout
```

### `service_test.go`

使用 fake User Repository 与 miniredis 覆盖登录、轮换、旧 token 重放拒绝、logout、错误密码和禁用用户。

## 7. `modules/user`

### `model.go`

`User` 是 GORM model；`UserAuth` 是认证查询数据；`CurrentUser` 是业务模型。

### `repository.go`

`Repository`/`NewRepository`、`Migrate`、`GetUserAuthByUsername`、`GetUserAuthByID`、`GetUserByID`、`CreateBootstrapUser` 承担全部 GORM 用户访问。

### `service.go`、`service_test.go`

`UserReader`、`Service.GetUserByID` 处理不存在和禁用规则；测试覆盖三种状态。

### `handler.go`、`routes.go`

`UserHandler.GetCurrentUser` 从 JWT Context 获取 ID，经 Service 回查并输出 `UserInfo`；`ToUserInfoResponse` 完成 DTO 转换。路由：

```text
GET /api/v1/users/me
GET /api/v1/auth/verify
```

## 8. `internal/router`

`routerall.go` 的 `AuthRouter`、`UserRouter` 在路由包内创建 Handler 并调用模块 `Register*Routes`。健康检查和 `/ping` 由 `cmd/api` 注册。`routerall_test.go` 覆盖登录、refresh Cookie、rotation、logout、当前用户、健康、Request ID 和未认证拒绝。

## 9. 构建与维护

| 文件 | 职责 |
| --- | --- |
| `backend/Dockerfile` | 构建 API、Gateway、healthcheck 两个 distroless target |
| `backend/.dockerignore` | 限定后端构建上下文 |
| `backend/go.mod`、`go.sum` | Gin、GORM、JWT、Redis 与测试依赖 |
| `.github/workflows/ci.yml` | 以 `backend/` 为 Go working directory，构建 `./backend` Docker context |

维护规则：Handler 不直接访问 GORM；Repository 不处理 HTTP/Cookie；refresh token 原值不进入日志或 Redis key；改变路由、模型、配置、测试或目录时同步本文和 `ARCHITECTURE.md`。
