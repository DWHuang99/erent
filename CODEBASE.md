# AI Gateway Codebase

本文是当前代码结构和维护入口索引。服务边界、调用链和一致性约束见 [ARCHITECTURE.md](ARCHITECTURE.md)。

## 1. Maven 多模块

```text
ai-gateway/
├─ pom.xml
├─ gateway-service/
│  ├─ pom.xml
│  ├─ Dockerfile
│  └─ src/
├─ core-service/
│  ├─ pom.xml
│  ├─ Dockerfile
│  └─ src/
├─ deploy/
│  ├─ prepare-nacos-env.sh
│  └─ nacos/
├─ nginx/
├─ monitoring/
├─ sql/
├─ docker-compose.yml
└─ docker-compose.prod.yml
```

根 `pom.xml`：

- packaging 为 `pom`；
- 聚合 `gateway-service`、`core-service`；
- 固定 Java 21、Spring Boot 3.5.16；
- 导入 Spring Cloud 2025.0.3 BOM；
- 导入 Spring Cloud Alibaba 2025.0.0.0 BOM；
- 导入 Testcontainers BOM；
- 统一 MyBatis Spring Boot Starter 版本。

常用 Maven 命令：

```bash
mvn clean verify
mvn -pl gateway-service test
mvn -pl core-service test
mvn -pl gateway-service -am package
mvn -pl core-service -am package
```

## 2. `gateway-service`

路径：

```text
gateway-service/src/main/java/com/example/aigateway/gateway/
```

| 类型 | 职责 |
| --- | --- |
| `GatewayServiceApplication` | WebFlux 边缘网关启动类 |
| `RequestIdGlobalFilter` | 校验或生成 `X-Request-Id`，清理可伪造的内部 Header，把请求 ID 写回响应 |

`RequestIdGlobalFilter`：

- 是 `GlobalFilter`，顺序为 `Ordered.HIGHEST_PRECEDENCE`；
- 只接受 `[A-Za-z0-9._:-]+`；
- 最大长度 128；
- 不合法或缺失时生成 UUID；
- 删除 `X-Internal-Token`、`X-User-Id`。

### Gateway 配置

`gateway-service/src/main/resources/application.yml`：

- `spring.application.name=gateway-service`；
- 从 Nacos Data ID `gateway-service.yml`、Group `AI_GATEWAY` 加载配置；
- 注册到 Nacos；
- 关闭 Discovery Locator，避免自动公开任意注册服务；
- `core-http` 默认路由到 `lb://core-service`，可由 `CORE_SERVICE_BASE_URL` 覆盖；
- `core-websocket` 默认路由到 `lb:ws://core-service`，可由 `CORE_SERVICE_WEBSOCKET_URL` 覆盖；
- 配置连接、响应超时和 `text/event-stream`；
- 暴露 health、info、prometheus。

显式路由路径：

```text
/api/**
/v1/**
/chat/**
/responses/**
/backend-api/codex/**
```

### Gateway 测试

| 测试 | 覆盖 |
| --- | --- |
| `RequestIdGlobalFilterTest` | 合法请求 ID 透传、非法值重建、内部 Header 清理和响应 Header |
| `GatewayRoutingIntegrationTest` | 实际 Spring Cloud Gateway 路由、LoadBalancer 服务实例解析和请求 ID 透传 |

## 3. `core-service`

启动类：

```text
core-service/src/main/java/com/example/aigateway/AiGatewayApplication.java
```

`core-service` 仍使用 Spring MVC 处理下游接口；Spring WebFlux 依赖主要用于非阻塞上游 `WebClient`。

### 3.1 `config`

| 类型 | 职责 |
| --- | --- |
| `RequestIdMdcFilter` | 从 `X-Request-Id` 读取或生成请求 ID，放入 MDC 并写回响应，完成后清理 |
| `SecurityConfig` | JWT/API Key Filter Chain、公开路径、密码编码和异常入口 |
| `ResponsesWebSocketConfig` | 注册 Responses WebSocket 路径和握手拦截 |

`core-service/src/main/resources/application.yml`：

- `spring.application.name=core-service`；
- 从 Nacos Data ID `core-service.yml`、Group `AI_GATEWAY` 加载配置；
- MySQL、Redis、RabbitMQ 连接；
- MyBatis Mapper；
- API Key/User/IP 固定窗口限流；
- 上游连接、读取、响应和整次请求超时；
- Provider Key 最大 failover 次数；
- Jasypt、JWT 和 Provider 默认地址；
- MDC 中 `requestId` 的日志格式；
- Actuator health、info、prometheus。

### 3.2 `controller`

| Controller | 主要路径 | 职责 |
| --- | --- | --- |
| `HealthController` | `GET /api/health` | 业务入口健康检查 |
| `AuthController` | `/api/auth/register`、`login`、`refresh` | 注册、登录、刷新 JWT |
| `UserController` | `GET /api/users/me` | 当前用户资料 |
| `WalletController` | `GET /api/wallets/me` | 当前用户钱包 |
| `ApiKeyController` | `/api/api-keys` | 创建、查询、更新和禁用平台 API Key |
| `ProviderKeyController` | `/api/provider-keys` | 创建和更新 Provider Key |
| `ModelController` | `GET /api/models` | 管理侧模型查询 |
| `GatewayModelController` | `GET /v1/models` | OpenAI 兼容模型列表 |
| `RequestLogController` | `GET /api/request-logs` | 当前用户日志分页 |
| `AdminController` | `GET /api/admin/health` | 管理侧健康占位 |
| `ModelCallController` | `/api/chat/**`、`/v1/**`、`/chat/**`、`/responses/**` | 非流式、SSE、Anthropic 和 Responses 兼容调用 |

`ModelCallController` 的协议路径：

```text
POST /api/chat/completions
POST /api/chat/completions/stream
POST /v1/chat/completions
POST /chat/completions
POST /v1/messages
POST /v1/messages/count_tokens
POST /v1/responses/**
POST /responses/**
POST /backend-api/codex/responses/**
GET  /v1/responses
GET  /responses
GET  /backend-api/codex/responses
```

最后三个 GET 用于未升级 WebSocket 时返回明确的 `426`。

### 3.3 `security`

| 类型 | 职责 |
| --- | --- |
| `ApiKeyAuthFilter` | 从 Bearer、`x-api-key`、`x-goog-api-key` 读取平台 API Key 并认证 |
| `ApiKeyPrincipal` | 已认证的 `apiKeyId/userId/prefix` |
| `ApiKeyHasher` | 平台 API Key hash 和验证 |
| `JwtAuthFilter` | 管理 API 的 JWT 认证 |
| `PasswordConfig` | 密码编码器 |
| `ProviderKeyCrypto` | Provider Key 静态加密与调用时解密 |
| `SensitiveDataMasker` | 生成不可逆的密钥显示提示 |

### 3.4 `ratelimit`

| 类型 | 职责 |
| --- | --- |
| `ApiKeyRateLimitFilter` | 顺序检查 API Key、用户和来源 IP 三个维度 |
| `FixedWindowRateLimiter` | Redis Lua 原子执行 `INCR + EXPIRE` |
| `GatewayRateLimitProperties` | 绑定 `gateway.rate-limit.*` |
| `RateLimitDecision` | 单次限流判断结果 |

Redis Key：

```text
rl:fixed:<dimension>:<identifier>:<window>s:<windowId>
```

### 3.5 `service`

| 类型 | 关键职责 |
| --- | --- |
| `ModelCallService` | 模型调用总编排、幂等、fingerprint、Key failover、流生命周期、用量和事件发布 |
| `BillingService` | 本地扣费事务、去重、钱包行锁、日志、用量、流水和幂等终态 |
| `ModelService` | 可用模型和当前价格规则查询 |
| `StreamResponseAccumulator` | 聚合文本、finish reason 和 usage；缺失 usage 时估算 |
| `UpstreamErrorService` | 统一映射上游错误并保留必要响应信息 |
| `ProviderKeyService` | Provider Key 创建、更新、所有权校验、加密和脱敏 |
| `ProviderCredentialService` | 解密 Key，组合 base URL、Header 和环境变量回退 |
| `ProviderKeySelectorService` | 查询并构造有序可用凭证，限制 failover 数量 |
| `ProviderKeyAvailabilityService` | 根据认证、限流、超载、超时和成功结果更新 Key 状态 |
| `ApiKeyService` | 平台 API Key 创建、认证、更新和禁用 |
| `JwtService` | JWT 签发与解析 |
| `RefreshTokenService` | Refresh Token 创建和验证；当前保存在单进程内存 |
| `CustomUserDetailsService` | Spring Security 用户加载 |
| `CurrentUserService` | 当前 JWT 用户解析 |
| `UserService` | 用户注册、角色关系、钱包初始化和用户查询 |
| `WalletService` | 钱包查询 |
| `RequestLogService` | 用户请求日志条件与分页查询 |

### 3.6 `gateway` 协议适配包

这里的 `core-service/.../gateway` 是“AI 协议适配层”，不要与独立的 `gateway-service` 混淆。

| 类型 | 职责 |
| --- | --- |
| `GatewayRequestAdapter` | OpenAI Chat、Anthropic Messages、Responses 请求归一化，并保留 OpenAI 原始 payload |
| `GatewayResponseAdapter` | 生成 OpenAI、Anthropic、Responses 和 Models 响应 |
| `GatewayStreamProtocol` | 三种下游流协议枚举 |
| `GatewayStreamFrame` | 单个事件名与 JSON data |
| `GatewayStreamSink` | SSE/WebSocket 共用发送边界 |
| `SseGatewayStreamSink` | 将 Frame 写入 `SseEmitter` |
| `GatewayStreamResponseAdapter` | 生成协议流事件、重放和错误帧 |
| `StreamReplayResponse` | 幂等缓存的聚合响应和精确 Frames |
| `ResponsesWebSocketHandler` | `response.create`、单 in-flight turn、连接内 history、取消和 WebSocket Frame |

### 3.7 `provider` 与 `client`

| 类型 | 职责 |
| --- | --- |
| `ProviderAdapter` | Provider 调用统一接口 |
| `ProviderAdapterFactory` | Provider code 到 Adapter 的注册表 |
| `ProviderCredential` | 一次调用使用的解密 Key、类型、base URL 和 Headers |
| `ProviderStreamEvent` | Provider 中立流事件 |
| `OpenAiAdapter` | OpenAI/OpenRouter 非流式和 SSE 调用 |
| `OpenAiStreamEventParser` | 解析 delta、finish reason、usage 和 DONE |
| `OpenaiRequest` / `OpenaiResponse` | OpenAI 上游请求与响应模型 |
| `ClaudeAdapter` / `ClaudeRequest` / `ClaudeResponse` | 原生 Anthropic Adapter 占位 |
| `GeminiAdapter` / `GeminiRequest` / `GeminiResponse` | 原生 Gemini Adapter 占位 |
| `UpstreamHttpClient` | 通用上游 HTTP JSON 和 SSE 传输 |
| `WebClientConfig` | 连接池、超时和 WebClient 配置 |

Provider 专属 parser 解释流 JSON；`UpstreamHttpClient` 不依赖具体模型协议。

### 3.8 `messaging`

| 类型 | 职责 |
| --- | --- |
| `RabbitTopology` | Topic Exchange、持久 Queue、Binding 和 JSON 转换器 |
| `GatewayEventPublisher` | 发布持久化 JSON 事件 |
| `RequestCompletedEvent` | 请求完成版本化事件 |
| `UsageRecordedEvent` | 用量已记录版本化事件 |
| `RequestLogConsumer` | 请求日志队列手动 ACK 骨架 |
| `UsageStatisticsConsumer` | 用量统计队列手动 ACK 骨架 |

当前 Consumer 只记录收到的事件，尚未写新的投影表。`ModelCallService` 只在非流式成功结果首次落库时发布两类事件。

### 3.9 `common` 与 `exception`

| 类型 | 职责 |
| --- | --- |
| `ErrorResponse` | 统一下游错误结构 |
| `BusinessException` | 带错误码和 HTTP 状态的业务异常 |
| `ProviderUpstreamException` | 保留上游错误上下文 |
| `GlobalExceptionHandler` | HTTP 异常到统一 JSON 的映射 |

### 3.10 DTO

查询投影：

- `ProviderModelPricing`
- `ModelUsageStats`
- `LowBalanceUser`
- `RequestLogQuery`
- `UserApiKeyQuery`
- `UserDailyUsageSummary`

请求 DTO：

- `ChatRequest`
- `CreateApiKeyRequest`
- `UpdateApiKeyRequest`
- `CreateProviderKeyRequest`
- `UpdateProviderKeyRequest`
- `LoginRequest`
- `UserRegister`

响应 DTO：

- `ChatResponse`
- `ApiKeyResponse`
- `CreateApiKeyResponse`
- `ProviderKeyResponse`
- `RequestLogResponse`
- `RequestLogPageResponse`
- `TokenResponse`
- `UserResponse`
- `WalletResponse`

### 3.11 Entity

| 类型 | 数据 |
| --- | --- |
| `UserAccount` | 用户账号 |
| `UserApiKey` | 用户与 API Key 联合查询投影 |
| `ApiKey` | 平台 API Key hash、prefix 和状态 |
| `ProviderKey` | Provider 凭证类型、地址、状态、优先级和冷却时间 |
| `ProviderKeyQuotaWindow` | Provider Key 配额窗口 |
| `Wallet` | 用户余额 |
| `WalletTransaction` | 钱包流水 |
| `RequestLog` | 请求、Provider、Key、模型、状态和耗时 |
| `UsageRecord` | 输入/输出 Token、来源、费用和实际 Provider Key |
| `IdempotencyRecord` | key hash、fingerprint、状态和缓存响应 |
| `UsageBillingDedup` | 请求扣费去重 claim |

### 3.12 Mapper 与 XML

Java 接口位于：

```text
core-service/src/main/java/com/example/aigateway/mapper/
```

同名 XML 位于：

```text
core-service/src/main/resources/mapper/
```

| Mapper | 主要操作 |
| --- | --- |
| `UserMapper` | 用户 CRUD、角色读取和绑定 |
| `ApiKeyMapper` | API Key CRUD、hash 查询和 last-used 更新 |
| `ModelMapper` | 可用模型、价格和模型统计 |
| `ProviderKeyMapper` | Key CRUD、候选调度和健康/冷却状态更新 |
| `ProviderKeyQuotaWindowMapper` | 配额窗口查询、upsert 和用量累计 |
| `WalletMapper` | 钱包插入、查询、`FOR UPDATE` 和余额更新 |
| `WalletTransactionMapper` | 钱包流水写入 |
| `RequestLogMapper` | 日志写入、查询、分页和统计 |
| `UsageRecordMapper` | Usage 写入和每日聚合 |
| `IdempotencyRecordMapper` | 幂等 claim、查询、锁定和终态更新 |
| `UsageBillingDedupMapper` | 扣费去重 claim、查询和锁定 |

修改 Mapper 方法签名时，必须同步修改 XML 的 `id`、参数名、ResultMap 和列名。

## 4. SQL

| 文件 | 用途 |
| --- | --- |
| `sql/schema.sql` | 当前完整数据库结构 |
| `sql/data.sql` | 本地学习数据 |
| `sql/20260705_idempotency_constraints.sql` | 幂等约束 |
| `sql/20260706_fingerprint_dedup.sql` | fingerprint 与扣费去重 |
| `sql/20260706_provider_key_scheduler.sql` | Provider Key 调度、状态和 quota window |
| `sql/20260711_streaming_usage_source.sql` | Usage 来源字段 |

增量脚本通过 `information_schema` 判断字段、索引和约束，设计为可重复执行。

## 5. 部署文件

| 文件 | 职责 |
| --- | --- |
| `gateway-service/Dockerfile` | 构建和运行边缘网关镜像 |
| `core-service/Dockerfile` | 构建和运行核心服务镜像 |
| `docker-compose.yml` | 本地/服务器公共编排、内部网络、健康检查和持久卷 |
| `docker-compose.prod.yml` | 用 `IMAGE_TAG` 替换为 GHCR 双镜像 |
| `deploy/prepare-nacos-env.sh` | 安全地向 `.env` 追加随机 Nacos 凭据和 Grafana 管理密码，不覆盖已有值 |
| `deploy/nacos/config/*.yml` | 两个服务的非敏感 Nacos Data ID 内容 |
| `deploy/nacos/publish-config.sh` | 登录 Nacos 并由一次性容器发布两个 Data ID |
| `deploy/k8s/gateway-service.yaml` | Namespace、Gateway ConfigMap、Deployment 与 ClusterIP Service |
| `deploy/k8s/core-service.yaml` | Core ConfigMap、Deployment 与 ClusterIP Service；只引用集群 Secret |
| `deploy/k8s/.env.secret.example` | 本地 Secret 变量模板，不包含真实值 |
| `deploy/k8s/README.md` | 本地 Kubernetes 最小迁移和验证步骤 |
| `nginx/default.conf` | 公网 HTTP、SSE 和 WebSocket 代理；其他路径返回 404 |
| `nginx/https.conf.example` | 未默认启用的 HTTPS 模板 |
| `monitoring/prometheus.yml` | Gateway 和 Core 指标抓取 |
| `.env.example` | 环境变量清单，不包含真实 Secret |
| `.github/workflows/ci.yml` | 测试、双镜像构建、兼容仓库规则的 `Docker build` 聚合检查、强制重发 Nacos 配置、发布、健康验证和按标签回滚 |

## 6. 测试索引

### `gateway-service`

| 测试 | 覆盖 |
| --- | --- |
| `RequestIdGlobalFilterTest` | 请求 ID 和内部 Header 信任边界 |
| `GatewayRoutingIntegrationTest` | Spring Cloud Gateway + LoadBalancer 路由 |

### `core-service`

| 测试 | 覆盖 |
| --- | --- |
| `AiGatewayApplicationTests` | 最小应用上下文 |
| `RequestIdMdcFilterTest` | 请求 ID 的 MDC、响应和清理 |
| `GatewayRequestAdapterTest` | 三类请求归一化 |
| `GatewayStreamResponseAdapterTest` | 三类流事件顺序 |
| `ResponsesWebSocketHandlerTest` | response.create、history、单 in-flight、失败和取消 |
| `OpenAiStreamEventParserTest` | delta、usage 和 DONE |
| `BillingServiceTest` | 成功、重复、冲突和部分计费 |
| `ModelCallServiceFingerprintTest` | JCS 与 route fingerprint |
| `ModelCallServiceProviderKeyFailoverTest` | 非流式 Key 切换和最终 Key 归因 |
| `ModelCallServiceStreamingTest` | 流式 failover、成功、部分计费和幂等重放 |
| `ModelCallServiceMessagingTest` | 首次发布、重复跳过和 MQ 异常隔离 |
| `ProviderKeyAvailabilityServiceTest` | 限流 reset 和认证错误状态 |
| `StreamResponseAccumulatorTest` | Provider usage 与估算 fallback |
| `UpstreamErrorServiceTest` | 上游错误映射 |
| `ApiKeyAuthFilterTest` | API Key Header 鉴权 |
| `ApiKeyRateLimitFilterTest` | 三维限流 Filter |
| `FixedWindowRateLimiterTest` | Redis Lua 固定窗口 |
| `RabbitMessagingSkeletonTest` | 拓扑、路由、消息属性和手动 ACK |
| `WalletServiceTest` | 钱包查询 |
| `UserMapperIntegrationTest` | Testcontainers MySQL/MyBatis 用户读写 |

## 7. 常见修改入口

| 需求 | 首先查看 |
| --- | --- |
| 修改公网路由 | `gateway-service/.../application.yml`、`nginx/default.conf` |
| 修改请求 ID 或内部 Header | `RequestIdGlobalFilter`、`RequestIdMdcFilter` |
| 修改 Nacos 配置 | 两个模块的 `application.yml`、`deploy/nacos/` |
| 新增兼容 API | `ModelCallController`、`GatewayRequestAdapter`、`GatewayResponseAdapter`，并增加 Gateway Route |
| 修改 SSE 事件 | `GatewayStreamResponseAdapter`、`SseGatewayStreamSink`、协议测试 |
| 修改 Responses WebSocket | `ResponsesWebSocketConfig`、`ResponsesWebSocketHandler`、Nginx/Gateway WebSocket 路由 |
| 修改流结束计费 | `ModelCallService`、`BillingService` |
| 修改幂等规则 | `ModelCallService`、`IdempotencyRecordMapper`、`UsageBillingDedupMapper` |
| 修改 Provider Key 选择 | `ProviderKeyMapper.xml`、`ProviderKeySelectorService` |
| 修改 Key 冷却 | `ProviderKeyAvailabilityService`、`ProviderKeyMapper.xml` |
| 新增 Provider | `ProviderAdapter`、新 Adapter、Provider 专属 parser 和测试 |
| 修改费用和钱包一致性 | `ModelMapper.xml`、`BillingService`、`WalletMapper.xml`、数据库约束 |
| 修改限流 | `GatewayRateLimitProperties`、`ApiKeyRateLimitFilter`、`FixedWindowRateLimiter` |
| 修改鉴权路径 | `ApiKeyAuthFilter`、`SecurityConfig`、Gateway 路由 |
| 修改消息事件 | `RabbitTopology`、`messaging/event`、`GatewayEventPublisher`、Consumer 和测试 |
| 修改容器发布 | 两个 Dockerfile、Compose、CI、`docs/nginx-deployment.md` |

## 8. 维护约束

- `gateway-service` 不加入 JDBC/MyBatis/业务 Entity 依赖。
- `core-service` 不依赖或回调 `gateway-service`。
- 新服务必须先有独立职责、数据所有权和接口合同。
- Nacos 不保存明文 Secret。
- 模型 POST 不添加网关自动重试。
- 调整外部路径时要同时检查 HTTP、SSE、WebSocket、Nginx 和 Gateway。
- 金额始终使用 `DECIMAL` / `BigDecimal`。
- 每次结构或行为变更都同步检查 `ARCHITECTURE.md` 与本文件。
