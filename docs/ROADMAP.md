# Go V2 Roadmap

## 已完成：单体认证基础

- 后端独立位于 `backend/`；
- Gin Router、Handler、Service、GORM Repository 分层；
- PostgreSQL 用户和 bcrypt 登录；
- HS256 access JWT 与 Bearer Filter；
- Redis refresh token、原子 rotation、HttpOnly Cookie 和 logout；
- `/users/me`、`/auth/verify` 与 PostgreSQL/Redis readiness；
- 单上游 Gateway 和根 Compose stack。

## 下一步：最小模型调用闭环

1. 平台 API Key；
2. 一个 Provider Credential 和 Provider Adapter；
3. `GET /v1/models`；
4. 非流式 `POST /v1/chat/completions`；
5. 最小请求日志和 Token 用量。

## 后续再考虑

- SSE、客户端断开取消、Provider failover；
- Redis 原子限流；
- 管理端 Provider/API Key 配置；
- 角色、权限和会话管理；
- 用量统计和审计。

## 明确延后

- 微服务、gRPC、服务发现和 K8s；
- JWKS、OIDC、OAuth；
- 钱包、真实货币计费、RabbitMQ；
- Responses、Anthropic Messages 和 WebSocket。
