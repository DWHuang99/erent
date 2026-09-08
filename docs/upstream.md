# OAuth upstream 开发与部署

## 授权与刷新流程

```text
Browser → /oauth/login?provider=oai → API 生成 state、PKCE 和授权地址 → OAuth provider
Browser → /oauth/callback → OauthService → TokenExchanger（directory）
        → UpstreamService.ExchangeCode → provider token endpoint
```

API 负责 Redis 登录流程、授权地址、ID token 验证、HTTP 错误映射和凭证加密持久化。directory 负责 deadline、protobuf 转换和 gRPC 错误转换；upstream 负责 provider 初始化、PKCE verifier 提交和有界的 token 兑换。连接由 API 的 applicationInstances 创建并关闭，路由装配将 directory 注入 service。

当前 API 与 upstream 都执行 OIDC discovery，因此两台机器都需要访问 issuer；token 兑换与刷新由 upstream 执行。两端应使用一致的 OAI issuer、client ID、client secret 与 redirect URL。Service 按 provider 保存多个实例；当前启动装配 oai。Login 校验 provider 并将其与用户、nonce、verifier 一起绑定到 Redis state；Callback 仅使用已保存的 provider，不接收回调 provider。

成功回调返回统一 JSON `{code:0,data:null,message:"oauth credentials saved"}`，不返回 provider token。SaveToken 验证 ID token 与 nonce，并加密保存到当前流程用户的 oauth_infos；scopes 包含 openid、profile、email、offline_access。

`GET /oauth/list` 仅返回 JWT 用户的账号元数据。`POST /oauth/refresh` 接收 `{id}`：Service 开启事务，Repository 按用户和 ID 加行锁查询，Service 解密既有 refresh token，经 directory 的 RefreshToken RPC 调用 upstream TokenSource；加密并保存新凭证后提交事务。可选 refresh/ID token 缺失时保留原值。它与本地 JWT refresh 接口相互独立；当前没有自动刷新任务。

## 配置

| 进程 | 变量 | 默认值 | 用途 |
| --- | --- | --- | --- |
| API | OAUTH_ENCRYPTION_KEY | 必填（启用 OAuth 时） | Base64 编码的 32 字节持久密钥，用于 AES-GCM 凭证加解密 |
| API | UPSTREAM_GRPC_TARGET | localhost:50051 | host:port，Compose 内固定为 upstream:50051 |
| API | UPSTREAM_GRPC_TIMEOUT | 10s | 单次 RPC 最大时长，沿用更短的调用方 deadline |
| upstream | UPSTREAM_GRPC_ADDR | :50051 | gRPC 监听地址 |
| upstream | UPSTREAM_OAUTH_TIMEOUT | 8s | 单次 provider token 请求最大时长 |
| upstream | UPSTREAM_SHUTDOWN_TIMEOUT | 10s | 收到 SIGTERM 后的排空时间，超时强制停止 |
| 两端 | OIDC_DISCOVERY_TIMEOUT | 10s | 启动阶段 discovery 超时 |
| 两端 | OAI_ISSUER / OAI_CLIENT_ID / OAI_CLIENT_SECRET / OAI_REDIRECT_URL | 空 | 整组为空禁用；secret 可按 provider 要求留空 |
| upstream | LOG_FILE | ./logs/backend/app.log | Compose 设置为独立的 upstream.log |

所有时长必须是正的 Go duration，如 8s；地址要求有效 host:port。API 只在 OAuth 启用时加载 gRPC 配置并创建连接。upstream 不读取 JWT、数据库或 Redis 配置。根 .env 供 Compose 插值，Go 进程本身不自动读取 .env。

upstream 提供标准 gRPC health 服务：空 service 检查进程是否在服务；upstream.UpstreamService 检查 OAI 是否成功初始化。OAuth 禁用时 upstream 进程仍健康，API 不检查 OAuth 依赖；OAuth 启用时 API readiness 额外检查 upstream.UpstreamService，失败返回 503。健康检查不触发新的外部 OAuth 请求，也不保证提供方此刻可用。

## 错误合同

| 情况 | gRPC code | Callback HTTP |
| --- | --- | --- |
| 缺少 code / verifier / provider | InvalidArgument | 400 |
| 授权码失效或被拒绝（invalid_grant） | Unauthenticated | 400 |
| provider 未配置 | FailedPrecondition | 503 |
| provider 网络故障、429 或 5xx | Unavailable | 503 |
| RPC 或 token 请求超时 | DeadlineExceeded | 504 |
| provider 配置错误、非法响应或其他兑换错误 | Internal | 502 |

state 缺失、过期或已消费仍返回 400，Redis 故障仍返回 500。directory 不把原始 gRPC 错误内容暴露给 handler，server 不记录 provider 响应体、code、verifier 或 token。Nginx 的 /oauth/ 不记录包含凭据的查询串。

授权码兑换与 refresh token 刷新均不配置应用重试或 gRPC retry policy。OAuth 客户端根据 discovery 声明选定 client_secret_basic / client_secret_post，公开客户端使用表单参数，避免自动探测认证方式时再次提交同一授权码。回调已消费 state 后失败，需要重新开始登录。

刷新接口同样映射上游超时和不可用错误；记录不存在或不属于当前用户返回 404，缺失/非法 ID 返回 400。刷新过程中可选的新 ID token 必须仍属于原账号。

## 本地运行

在根 .env 填入 provider 配置后：

```powershell
docker compose --env-file .env -f backend/docker-compose.yml up --build -d
docker compose --env-file .env -f backend/docker-compose.yml ps -a
docker compose --env-file .env -f backend/docker-compose.yml logs -f api upstream
docker compose --env-file .env -f backend/docker-compose.yml exec upstream /grpc-healthcheck
```

本地 Stack 新增 upstream 容器及 upstream-logs volume，50051 只暴露在 Compose 网络内，不映射宿主机。默认使用该网络内的明文 gRPC。API 等待 upstream 健康；upstream 的 stop_grace_period 为 15s，修改排空超时时需同步确保容器停止宽限期更长。

Nginx 与 Vite 均将 /oauth/ 转发到 Gateway。控制台通过携带 Bearer 凭证的 `GET /oauth/login?provider=oai` 获取授权链接（`Accept: application/json`）；直接在地址栏访问不携带该凭证。

`OAI_REDIRECT_URL` 必须匹配所用客户端注册的回调地址，不能随意替换成网关地址。当前 Codex 本地登录配置使用 `http://localhost:1455/auth/callback`；API 与 upstream 必须使用相同的值，修改后需重新创建这两个容器。项目未监听 1455 端口：授权后如本地回调页面无法打开，将浏览器地址栏中的完整回调 URL 粘贴到 OAuth 页的手动回调入口，由前端校验本次 state 后提交到 `/oauth/callback`。不要在另一个 Codex 登录流程同时占用该端口时操作，也不要手动修改已生成授权链接中的 redirect_uri。

直接运行时，先导入环境变量，在两个终端分别执行：

```powershell
go -C backend run ./cmd/upstream
go -C backend run ./cmd/api
```

直接运行 API 仍需要已迁移的 PostgreSQL 和 Redis。

## 跨服务器部署预留

程序支持双向 TLS。启用时，每个进程设置：

- UPSTREAM_GRPC_TLS_CA_FILE：信任的服务专用 CA；
- UPSTREAM_GRPC_TLS_CERT_FILE / UPSTREAM_GRPC_TLS_KEY_FILE：该进程的证书与私钥；
- API 的 UPSTREAM_GRPC_TLS_SERVER_NAME：需要覆盖目标地址校验名称时使用，必须匹配服务端证书 SAN。

前三项必须一起设置，文件或证书无效会拒绝启动，TLS 配置错误不会降级到明文。服务端强制验证客户端证书，客户端验证服务器证书；证书应只授予被授权的服务身份。API 使用 ClientAuth 证书，upstream 使用 ServerAuth 证书。探针可设置 UPSTREAM_HEALTHCHECK_TLS_CERT_FILE / UPSTREAM_HEALTHCHECK_TLS_KEY_FILE 指向独立的 ClientAuth 身份，并为探针设置 UPSTREAM_GRPC_TARGET 和 UPSTREAM_GRPC_TLS_SERVER_NAME；未指定探针身份时使用当前进程证书，该证书需要同时允许 ClientAuth。

本地 Compose 不挂载证书；跨服务器部署需要在部署配置中只读挂载证书并传入相应变量，使用私网或 mTLS 限定 API 到 upstream 的访问。真实服务器地址、证书、SSH secrets、CD 和新的 GitHub Environment 等服务器确定后在 erent-deploy 中配置，本轮未修改该仓库或 GitHub 设置。

## 构建与生成

backend/Dockerfile 提供 upstream 独立 target，镜像包含 /upstream 和 /grpc-healthcheck，使用非 root 用户。CI 构建此 target；发布工作流会发布 ghcr.io/dwhuang99/erent-upstream:<source-sha>。新服务器上线时使用与 API 一致的源码版本。

协议源文件是 backend/proto/upstream.proto，生成代码提交在 backend/internal/rpc/upstream。工具版本：protoc 35.1、protoc-gen-go v1.36.12、protoc-gen-go-grpc v1.6.2。安装工具到 PATH 后：

```powershell
./.scripts/update-grpc.ps1
go -C backend test ./...
go -C backend vet ./...
go -C backend build ./cmd/...
```

集成测试使用真实 gRPC 编解码和模拟 OIDC/token HTTP 服务，覆盖 callback → service → directory → server、PKCE、token 类型/有效期、错误映射、state 单次消费、deadline 与无重复兑换；mTLS 测试覆盖合法身份、缺少客户端证书、错误服务名和不受信任 CA。
