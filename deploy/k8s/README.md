# Kubernetes 最小迁移

当前清单只把 `gateway-service` 和 `core-service` 部署到 Kubernetes。MySQL、Redis 和 RabbitMQ 暂时继续由 Docker Compose 运行。

## 配置模式

| 运行方式 | 服务发现 | Gateway 到 Core |
| --- | --- | --- |
| Docker Compose | Nacos | `lb://core-service` / `lb:ws://core-service` |
| Kubernetes | Kubernetes Service DNS | `http://core-service:8081` / `ws://core-service:8081` |

Kubernetes 资源统一放在 `ai-gateway` Namespace。Namespace 和 Gateway ConfigMap 位于 `gateway-service.yaml`，Core ConfigMap 位于 `core-service.yaml`。两个 ConfigMap 都会关闭 Nacos，并提供服务地址或非敏感连接参数；敏感信息由集群中的 `core-service-secret` 提供，不写入可提交的 YAML。

## 使用前准备

1. 检查被 Git 忽略的 `deploy/k8s/.env.secret`。当前文件使用 Compose 的本地开发默认值，只适合本机学习。
2. 构建两个本地镜像，并按所用集群的方式加载镜像。
3. 启动 Compose 中的外部依赖：

```powershell
$env:MIDDLEWARE_BIND_ADDRESS = "0.0.0.0"
docker compose up -d mysql redis rabbitmq
```

`0.0.0.0` 只用于本地迁移练习，使 Pod 能访问宿主机端口。请确认防火墙没有把 `3306`、`6379` 和 `5672` 暴露给不可信网络；练习结束后恢复为默认的 `127.0.0.1`。

`core-service.yaml` 中的 ConfigMap 当前使用 minikube 的 `host.minikube.internal`。如果使用其他 Kubernetes 环境，请替换为 Pod 实际能够访问的宿主机或外部中间件地址。

## 创建 Secret 并应用清单

Namespace 只定义一次，位于 `gateway-service.yaml`。先创建 Namespace，再从本地文件创建 Secret，最后应用 Core：

```powershell
kubectl apply -f deploy/k8s/gateway-service.yaml

kubectl create secret generic core-service-secret `
  --namespace ai-gateway `
  --from-env-file=deploy/k8s/.env.secret `
  --dry-run=client `
  -o yaml |
kubectl apply -f -

kubectl apply -f deploy/k8s/core-service.yaml
kubectl get all -n ai-gateway
kubectl get configmap -n ai-gateway
```

修改 `.env.secret` 后需要重新执行 Secret 创建命令，并重启通过环境变量读取 Secret 的 Core Pod：

```powershell
kubectl rollout restart deployment/core-service -n ai-gateway
```

## 环境隔离

本地、测试和生产环境使用相同的变量名，但必须使用不同的值：

```text
本地：deploy/k8s/.env.secret
测试：由测试环境 CI/CD Secret 或受保护的 .env.test.secret 提供
生产：由生产环境 CI/CD Secret 或外部 Secret Manager 提供
```

所有真实 Secret 文件都不能提交 Git。生产环境不建议把 `.env.prod.secret` 保存在开发者电脑上；应优先使用 GitHub Environment Secrets 或云端 Secret Manager，并为测试、生产使用不同的 Namespace、数据库账号、RabbitMQ 账号、JWT 和 Jasypt 密钥。

查看运行状态和日志：

```powershell
kubectl get pods -n ai-gateway
kubectl logs -n ai-gateway deployment/gateway-service
kubectl logs -n ai-gateway deployment/core-service
```

ClusterIP 只允许集群内部访问。第一次验证可以使用端口转发：

```powershell
kubectl port-forward -n ai-gateway service/gateway-service 8080:8080
curl.exe --noproxy "*" http://localhost:8080/api/health
```

在 Gateway Pod 内，`core-service` 会被 Kubernetes DNS 解析为同一 Namespace 下的 ClusterIP Service，不再需要 Nacos 提供实例列表。
