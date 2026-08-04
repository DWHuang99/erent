# Nginx 与 Linux 部署

本文说明当前双服务版本如何通过 Docker Compose、Nacos 和 Nginx 部署。测试服务器地址：

```text
106.53.192.153
```

当前 HTTP 入口：

```text
http://106.53.192.153:8088
```

## 1. 运行时链路

```text
Client
  -> host :8088
  -> Nginx :80
  -> gateway-service :8080
  -> Nacos discovery
  -> core-service :8081
```

Nginx 不再直接转发到核心业务服务。`gateway-service` 根据 Nacos 中注册的 `core-service` 实例完成转发。

## 2. 网络边界

公网只开放：

```text
8088 -> nginx:80
```

正式配置域名和证书后，公网入口应改为：

```text
80 / 443 -> Nginx
```

以下端口不能绑定到公网 `0.0.0.0`：

- MySQL 3306
- Redis 6379
- RabbitMQ 5672 / 15672
- Nacos 客户端和控制台端口
- Gateway 8080
- Core 8081
- Prometheus 9090
- Grafana 3000

需要查看管理界面时，通过宿主机回环绑定或 SSH 隧道访问。例如：

```bash
ssh -L 18080:127.0.0.1:18080 deploy@106.53.192.153
```

随后在本机访问 `http://127.0.0.1:18080`。当前回环绑定为：

```text
127.0.0.1:8848  -> Nacos client / admin API
127.0.0.1:9848  -> Nacos gRPC
127.0.0.1:18080 -> Nacos console
127.0.0.1:15672 -> RabbitMQ management
127.0.0.1:9090  -> Prometheus
127.0.0.1:3000  -> Grafana
```

## 3. Nginx 路由

`nginx/default.conf` 负责：

- 普通 HTTP API；
- HTTP SSE；
- Responses WebSocket Upgrade；
- 可信的转发 Header。

需要转发的业务前缀：

```text
/api/
/v1/
/chat/
/responses/
/backend-api/codex/
```

请求始终先进入 `gateway-service`，不能为某些“方便调试”的路径绕过网关直连 Core。

### 来源 IP

Nginx 应把 `X-Forwarded-For` 设置为它实际看到的 `$remote_addr`，不要直接把客户端传入的 `X-Forwarded-For` 拼到链首。`core-service` 的 IP 限流依赖可信代理提供的来源地址。

### SSE

SSE 链路要求 Nginx：

```nginx
proxy_http_version 1.1;
proxy_buffering off;
proxy_cache off;
proxy_read_timeout 3600s;
gzip off;
```

Spring Cloud Gateway 已把 `text/event-stream` 声明为 streaming media type。验证 SSE 时使用 `curl -N`：

```bash
curl -N \
  -H "Authorization: Bearer <platform-api-key>" \
  -H "Content-Type: application/json" \
  http://127.0.0.1:8088/v1/chat/completions \
  -d '{"model":"your-model","stream":true,"messages":[{"role":"user","content":"hello"}]}'
```

应该逐条看到事件，而不是请求结束后一次性收到所有内容。

### WebSocket

Responses WebSocket 使用：

```text
/v1/responses
/responses
/backend-api/codex/responses
```

Nginx 必须转发 `Upgrade` 和 `Connection`。Gateway 的 `core-websocket` 路由再通过 `lb:ws://core-service` 转发。普通 GET 访问这些路径会由 Core 返回 `426 Upgrade Required`，这是预期行为。

## 4. 环境变量

在服务器 `/opt/ai-gateway/.env` 中保存部署变量。先参考 `.env.example`，至少替换：

```text
MYSQL_ROOT_PASSWORD
DB_PASSWORD
RABBITMQ_PASSWORD
JWT_SECRET
JASYPT_PASSWORD
GRAFANA_ADMIN_PASSWORD
```

规则：

- `.env` 不提交 Git；
- 不在 Nacos Data ID 中保存密码和 Provider API Key；
- 不使用示例默认密码；
- `JASYPT_PASSWORD` 必须保持稳定，否则已有 Provider Key 无法解密；
- 生产服务器的 `.env` 应限制为部署用户可读。

Linux 上可以用以下脚本向 `.env` 追加缺失的 Nacos 参数和 Grafana 管理密码：

```bash
./deploy/prepare-nacos-env.sh .env
```

脚本不会覆盖已经存在的参数。生产覆盖文件还会强制检查数据库、RabbitMQ、JWT、Jasypt 和 Grafana密钥，缺失时部署直接停止。若 `.env` 来自 `.env.example`，应先手动替换所有 `replace-with-...` 占位符。

## 5. 本地 Compose 验证

```powershell
Copy-Item .env.example .env
docker compose config --quiet
docker compose up -d --build
docker compose ps
```

观察服务注册与启动：

```powershell
docker compose logs --tail=100 nacos
docker compose logs --tail=100 gateway-service
docker compose logs --tail=100 core-service
```

验证 Nginx 到 Gateway 再到 Core：

```powershell
curl.exe --noproxy "*" http://localhost:8088/api/health
```

验证请求 ID：

```powershell
curl.exe -i --noproxy "*" `
  -H "X-Request-Id: deploy-smoke-001" `
  http://localhost:8088/api/health
```

响应应包含：

```text
X-Request-Id: deploy-smoke-001
```

## 6. Nacos 配置

Compose 启动时使用：

| Service | Data ID | Group |
| --- | --- | --- |
| `gateway-service` | `gateway-service.yml` | `AI_GATEWAY` |
| `core-service` | `core-service.yml` | `AI_GATEWAY` |

相关内容位于：

```text
deploy/nacos/
```

`nacos-config-init` 是一次性 `curlimages/curl` 容器。它等待 Nacos 健康、使用管理员账号登录，然后执行 `deploy/nacos/publish-config.sh` 发布两个 Data ID。Gateway 和 Core 都依赖该容器成功退出后再启动。

CI/CD 每次部署都会先等待 Nacos 健康，再以 `--force-recreate` 重新运行该容器。这样即使只修改了挂载的 `deploy/nacos/config/*.yml`，新内容也会重新发布，而不会复用上一次已经退出的容器。

Nacos 当前启用客户端、管理 API 和控制台认证。应用进程使用 `NACOS_USERNAME` 与 `NACOS_PASSWORD` 连接；Compose 会把 `.env` 中的 `NACOS_ADMIN_PASSWORD` 映射为两个应用的 `NACOS_PASSWORD`。服务端 Token、服务身份和管理员密码都来自 `.env`。

可动态调整的内容只包括非敏感参数，例如网关超时、限流窗口和 Provider failover 次数。

修改 Nacos 配置后，要区分两类参数：

- 支持刷新且绑定到可刷新的配置对象：可以运行时生效；
- 端口、核心 Spring 容器结构和部分 Gateway 路由：需要重启对应服务确认。

配置中心不可用时，已经运行的服务可能继续使用内存中的旧配置，但新实例启动和服务发现可能失败。当前 Nacos 是 standalone，不应被描述为高可用。

## 7. 手动部署

服务器仓库目录：

```text
/opt/ai-gateway
```

在更新前先保存当前成功运行的镜像标签。然后：

```bash
cd /opt/ai-gateway
git checkout main
git pull --ff-only origin main

export IMAGE_TAG=<已由 CI 推送的 commit SHA>

docker compose \
  -f docker-compose.yml \
  -f docker-compose.prod.yml \
  config --quiet

docker compose \
  -f docker-compose.yml \
  -f docker-compose.prod.yml \
  pull gateway-service core-service

docker compose \
  -f docker-compose.yml \
  -f docker-compose.prod.yml \
  up -d --wait --wait-timeout 180 nacos

docker compose \
  -f docker-compose.yml \
  -f docker-compose.prod.yml \
  up \
  --no-deps \
  --force-recreate \
  --exit-code-from nacos-config-init \
  nacos-config-init

docker compose \
  -f docker-compose.yml \
  -f docker-compose.prod.yml \
  up -d --no-build --remove-orphans
```

检查：

```bash
docker compose \
  -f docker-compose.yml \
  -f docker-compose.prod.yml \
  ps

docker compose \
  -f docker-compose.yml \
  -f docker-compose.prod.yml \
  exec -T nginx nginx -t

docker compose \
  -f docker-compose.yml \
  -f docker-compose.prod.yml \
  exec -T nginx nginx -s reload
```

健康验证：

```bash
curl -fsS --retry 24 --retry-delay 5 \
  http://127.0.0.1:8088/api/health

curl -fsS --retry 12 --retry-delay 5 \
  http://106.53.192.153:8088/api/health
```

失败时优先查看：

```bash
docker compose \
  -f docker-compose.yml \
  -f docker-compose.prod.yml \
  logs --tail=200 gateway-service core-service nacos nginx
```

## 8. GitHub Actions 部署

`.github/workflows/ci.yml`：

1. 在 `develop`、`main` push 和 Pull Request 上执行 `mvn clean verify`；
2. 分别构建 `gateway-service`、`core-service`；
3. 两个矩阵任务成功后生成仓库规则要求的 `Docker build` 聚合检查；
4. `main` push 时把两个 SHA 标签镜像推到 GHCR；
5. 进入 `erent` GitHub Environment 审批；
6. SSH 到服务器并执行 Compose 更新；
7. 执行 `nginx -t` 和 reload；
8. 验证服务器回环入口和公网入口。

镜像：

```text
ghcr.io/dwhuang99/erent-gateway-service:<commit-sha>
ghcr.io/dwhuang99/erent-core-service:<commit-sha>
```

两个服务必须使用同一个 commit SHA，避免协议或配置版本错配。

## 9. 回滚

推荐使用 GitHub Actions 的 `workflow_dispatch`：

1. 打开 `CI` 工作流；
2. 选择 Run workflow；
3. `image_tag` 填写以前测试、构建、部署成功的 commit SHA；
4. 通过 `erent` 环境审批；
5. 等待双服务和公网健康检查成功。

手动回滚：

```bash
cd /opt/ai-gateway
export IMAGE_TAG=<previous-good-sha>

docker compose \
  -f docker-compose.yml \
  -f docker-compose.prod.yml \
  pull gateway-service core-service

docker compose \
  -f docker-compose.yml \
  -f docker-compose.prod.yml \
  up -d --no-build gateway-service core-service

curl -fsS http://127.0.0.1:8088/api/health
```

回滚应用镜像不会自动回滚数据库。涉及不可向后兼容的数据库迁移时，必须提前准备独立的恢复方案。

## 10. 启用 HTTPS

`nginx/https.conf.example` 只是模板，默认不加载。

启用步骤：

1. 把域名解析到 `106.53.192.153`；
2. 在安全组和防火墙开放 80、443；
3. 申请证书并把证书、私钥放到服务器受保护目录；
4. 复制 HTTPS 模板为实际 `.conf` 并替换域名和证书路径；
5. 在 Compose 发布 `80:80`、`443:443` 并只读挂载证书；
6. 运行 `nginx -t`；
7. 重载后验证普通 API、SSE 和 WebSocket；
8. 最后再启用 HTTP 到 HTTPS 跳转。

证书私钥不能提交 Git，也不能放进 Nacos。

## 11. 发布后检查清单

```text
[ ] Nginx 是唯一公网业务入口
[ ] gateway-service 和 core-service 都健康
[ ] Nacos 中能看到两个服务实例
[ ] gateway-service 能通过服务名访问 core-service
[ ] /api/health 经公网返回成功
[ ] X-Request-Id 能在响应和 Core 日志中对应
[ ] SSE 逐条输出
[ ] Responses WebSocket 能完成 Upgrade
[ ] MySQL / Redis / RabbitMQ / Nacos 未暴露公网
[ ] 两个应用镜像使用同一个 SHA
[ ] 已记录上一版可回滚 SHA
```
