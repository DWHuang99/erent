# Deployment boundary

生产部署拆分为公开源码仓库和私有部署仓库：

```text
DWHuang99/erent (public)
  CI -> tests and build verification
  Release images -> GHCR images tagged with the source commit SHA
                         |
                         v
DWHuang99/erent-deploy (private)
  production Compose + GitHub Environment + manual CD
                         |
                         v
                    Linux server
```

## Published artifacts

`.github/workflows/release.yml` 在 `main` push、`v*` tag 或手动触发时发布：

```text
ghcr.io/dwhuang99/erent-api:<source-sha>
ghcr.io/dwhuang99/erent-gateway:<source-sha>
ghcr.io/dwhuang99/erent-upstream:<source-sha>
ghcr.io/dwhuang99/erent-migrations:<source-sha>
ghcr.io/dwhuang99/erent-web:<source-sha>
```

本公开仓库发布五个镜像，接入 upstream 时 API 与 upstream 应使用同一个源码版本。`latest` 只表示公开仓库默认分支的最新构建，生产部署和回滚都应使用不可变的完整源码 SHA 或明确的 release tag。

`erent-migrations` 继承 `migrate/migrate` CLI 并携带该源码版本的 `backend/migrations`，使数据库 schema 与 API/Gateway/Web 版本保持一致。Web 镜像包含 Vue `dist` 和 Nginx；生产服务器不需要 Go 或 Node.js 构建环境。

## Private deployment repository

私有仓库地址为 `https://github.com/DWHuang99/erent-deploy`。它保存：

```text
.github/workflows/cd.yml
docker-compose.yml
.env.example
README.md
```

它不复制业务源码，也不保存生产 `.env`、SSH key、数据库备份或 TLS 私钥。CD 使用 `workflow_dispatch` 的 `image_tag` 输入部署指定源码 SHA；部署失败或需要回滚时，重新输入此前发布成功的 SHA。

## Manual security setup

需要在私有仓库手动完成：

1. 创建或填写 `production` Environment 的 SSH secrets 与服务器 variables；
2. 在四个 GHCR Package 的 **Manage Actions access** 中授予 `erent-deploy` Read；
3. 为服务器配置只读 Deploy Key，并在部署目录创建不提交的生产 `.env`；
4. 可选地为 `production` 添加 Required reviewers，并通过服务器现有 HTTPS 反向代理公开 Web。

全部字段和首次部署命令见私有仓库 `README.md`。

## Upstream rollout pending

当前只在公开源码仓库完成 upstream 进程、可选 mTLS、本地 Compose、镜像构建与发布配置。私有仓库现有的四镜像部署流程保持原状；upstream 新服务器到位后，再配置独立的 GitHub Environment、SSH/证书、服务器变量、生产 Compose 与 CD，并授予 erent-upstream package 的读取权限。源码中不填写未知的服务器地址，也不创建生产 Environment。程序配置、探针身份和网络边界见 [upstream.md](upstream.md)。
