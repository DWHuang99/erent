# AI Gateway

AI Gateway 是一个用于学习 AI API 聚合、用量计费和 Spring Cloud 工程化的项目。当前版本已经从单个 Spring Boot 进程调整为一个最小、可部署的双服务结构：

```text
Client
  -> Nginx
  -> gateway-service（Spring Cloud Gateway / WebFlux）
  -> Nacos 发现 core-service
  -> core-service（Spring MVC + 原有业务）
  -> MySQL / Redis / RabbitMQ / AI Provider
```

这次拆分的目标是先建立真实的服务注册、发现、配置和网关调用链，而不是一次性把所有业务拆成大量小服务。

## 当前能力

- 用户注册、登录、JWT 与平台 API Key 管理
- Provider Key 加密保存、调度、调用前故障切换和健康状态维护
- OpenAI Chat Completions、Anthropic Messages、OpenAI Responses 兼容接口
- HTTP SSE 与 Responses WebSocket 流式调用
- 请求日志、Token 用量、价格计算、钱包并发扣减和幂等
- Redis 固定窗口限流
- RabbitMQ 请求完成与用量事件骨架
- Nacos 服务注册、发现和非敏感配置加载
- Nginx 单一公网入口
- Prometheus / Grafana 基础监控
- GitHub Actions 自动测试、构建双镜像和 Linux 部署

## 服务边界

| 模块 | 运行时服务 | 职责 |
| --- | --- | --- |
| `gateway-service` | `gateway-service` | 公网 API 路由、服务发现、HTTP/SSE/WebSocket 转发、请求 ID 生成与透传、清理客户端伪造的内部 Header |
| `core-service` | `core-service` | 鉴权、限流、用户、API Key、Provider 调用、SSE/WebSocket 业务、用量、计费、MyBatis、RabbitMQ |

Provider、Billing 和 User 暂时没有继续拆分：

- Provider 调用包含原始请求透传、调用前 Key failover、SSE/WebSocket 生命周期和流结束计费，当前还没有稳定的跨服务协议。
- Billing 同时更新钱包、请求日志、用量、流水和幂等状态，依赖一个本地数据库事务。
- 用户注册同时创建用户、角色关系和钱包。

如果现在强行拆开，会立刻引入分布式事务、跨服务幂等和复杂流式协议，却不会明显改善当前项目。后续应先明确数据归属和接口契约，再逐个拆分。

## 技术版本

- JDK 21
- Spring Boot 3.5.16
- Spring Cloud 2025.0.3
- Spring Cloud Alibaba 2025.0.0.0
- Nacos 3.2.3
- MyBatis 3.0.5
- MySQL 8.4
- Redis 7.4
- RabbitMQ 4
- Maven 多模块

根 `pom.xml` 通过 Spring Cloud、Spring Cloud Alibaba 和 Testcontainers BOM 统一管理兼容版本。不要在子模块中单独覆盖 Spring Cloud 组件版本。

## 项目结构

```text
ai-gateway/
├─ pom.xml                         Maven 聚合父工程和依赖 BOM
├─ gateway-service/
│  ├─ Dockerfile
│  ├─ pom.xml
│  └─ src/
├─ core-service/
│  ├─ Dockerfile
│  ├─ pom.xml
│  └─ src/
├─ deploy/
│  ├─ prepare-nacos-env.sh         生成 Nacos 随机认证参数
│  ├─ k8s/                         双服务 Kubernetes 最小迁移清单
│  └─ nacos/
│     ├─ publish-config.sh         一次性发布 Data ID
│     └─ config/                   两个服务的非敏感配置
├─ nginx/                          HTTP 入口与 HTTPS 示例
├─ monitoring/                     Prometheus 配置
├─ sql/                            全量建库与增量脚本
├─ docker-compose.yml
├─ docker-compose.prod.yml
├─ ARCHITECTURE.md
├─ CODEBASE.md
└─ docs/
```

## 配置约定

两个服务都通过 Nacos 注册和发现。配置中心默认使用以下配置：

| Service | Data ID | Group |
| --- | --- | --- |
| `gateway-service` | `gateway-service.yml` | `AI_GATEWAY` |
| `core-service` | `core-service.yml` | `AI_GATEWAY` |

Nacos 只适合保存可公开给应用进程的非敏感运行参数，例如超时、限流和 Provider failover 次数。以下内容必须由服务器环境变量或受保护的 CI/CD Secret 注入：

- `MYSQL_ROOT_PASSWORD`
- `DB_PASSWORD`
- `RABBITMQ_PASSWORD`
- `JWT_SECRET`
- `JASYPT_PASSWORD`
- `GRAFANA_ADMIN_PASSWORD`
- Provider API Key

尤其要保持 `JASYPT_PASSWORD` 稳定；改变它以后，数据库里既有的加密 Provider Key 将无法解密。

## 本地运行

首次使用先从示例创建本地环境文件，并替换所有占位值：

```powershell
Copy-Item .env.example .env
```

Linux 上也可以用 `deploy/prepare-nacos-env.sh .env` 向现有 `.env` 追加缺失的 Nacos 认证参数和 Grafana 管理密码。脚本不会覆盖已经存在的值；生产覆盖文件会拒绝缺少核心数据库、RabbitMQ、JWT、Jasypt 或 Grafana 密钥的部署。

启动完整环境：

```powershell
docker compose up -d --build
docker compose ps
```

验证公网入口对应的本地链路：

```powershell
curl.exe --noproxy "*" http://localhost:8088/api/health
```

查看关键服务日志：

```powershell
docker compose logs -f gateway-service core-service nacos nginx
```

只运行测试：

```powershell
mvn clean verify
```

分别启动某个模块：

```powershell
mvn -pl core-service spring-boot:run
mvn -pl gateway-service spring-boot:run
```

直接用 Maven 启动时，需要先准备 MySQL、Redis、RabbitMQ 和 Nacos，并设置：

```text
NACOS_SERVER_ADDR
NACOS_DISCOVERY_ENABLED=true
NACOS_CONFIG_ENABLED=true
```

`gateway-service` 使用 `lb://core-service` 路由，因此没有 Nacos 或等价的服务实例列表时，它不能找到核心服务。

Gateway 的目标地址可以由环境变量覆盖：

```text
CORE_SERVICE_BASE_URL
CORE_SERVICE_WEBSOCKET_URL
```

Compose 显式使用 Nacos 负载均衡 URI；`deploy/k8s/gateway-service.yaml` 中的 ConfigMap 则关闭 Nacos，并把它们改为同一 Namespace 下的 `core-service` ClusterIP Service。最小 Kubernetes 迁移步骤见 `deploy/k8s/README.md`。

## 路由与请求 ID

Nginx 把以下业务路径交给 `gateway-service`：

```text
/api/**
/v1/**
/chat/**
/responses/**
/backend-api/codex/**
```

网关会：

1. 接受符合 `[A-Za-z0-9._:-]+` 且不超过 128 字符的 `X-Request-Id`；
2. 缺失或不合法时生成 UUID；
3. 删除客户端传入的 `X-Internal-Token` 和 `X-User-Id`；
4. 把 `X-Request-Id` 传给 `core-service` 并写回响应。

`core-service` 把该值放入 MDC，所以可以用同一个请求 ID 串联网关响应与核心服务日志。

## Linux 部署与 CI/CD

当前测试服务器入口：

```text
http://106.53.192.153:8088
```

CI/CD 行为：

1. `develop`、`main` 的 push 和 Pull Request 执行 `mvn clean verify`；
2. `main` 测试通过后分别构建并推送两个镜像：
   - `ghcr.io/dwhuang99/erent-gateway-service:<commit-sha>`
   - `ghcr.io/dwhuang99/erent-core-service:<commit-sha>`
3. 经 GitHub Environment `erent` 审批后，通过 SSH 在 `/opt/ai-gateway` 更新 Compose；
4. 校验并重载 Nginx；
5. 同时检查服务器本机和公网健康接口。

验证：

```bash
curl -fsS http://127.0.0.1:8088/api/health
curl -fsS http://106.53.192.153:8088/api/health
```

回滚时在 GitHub Actions 手动运行 `CI` 工作流，填写一个以前成功发布过的 commit SHA 作为 `image_tag`。工作流会让两个服务使用同一个标签，避免网关和核心服务版本错配。

更完整的 Nginx、SSE、WebSocket、部署和回滚命令见 [docs/nginx-deployment.md](docs/nginx-deployment.md)。

## 网络安全边界

公网只应开放 Nginx 的 `8088`（正式启用域名和 HTTPS 后改为 `80/443`）。以下服务只在 Docker 内部网络或宿主机回环地址访问，不能直接暴露公网：

- MySQL
- Redis
- RabbitMQ AMQP 与管理端
- Nacos 客户端端口与控制台
- `gateway-service`
- `core-service`
- Prometheus / Grafana 管理入口

## 当前限制

- `RefreshTokenService` 仍把 Refresh Token 保存在单进程内存中，因此 `core-service` 暂时只能运行一个副本；进程重启后旧 Refresh Token 失效。
- RabbitMQ 发布位于核心数据库事务提交之后，目前是 best-effort。Broker 故障时核心调用仍可成功，但事件可能丢失；后续使用 Transactional Outbox 解决。
- Nacos 当前是 standalone 单节点，适合学习和单机预发布，不具备高可用能力。
- Provider、Billing、User 仍共享 `core-service` 和一个数据库，这是为了保留当前本地事务与流式契约。
- HTTPS 示例尚不代表证书已经部署；公网正式使用前必须配置域名、证书、强密码、备份和防火墙。
