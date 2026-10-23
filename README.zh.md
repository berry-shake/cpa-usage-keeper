<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="./assets/keeper-logo-dark.svg" />
    <source media="(prefers-color-scheme: light)" srcset="./assets/keeper-logo-light.svg" />
    <img src="./assets/keeper-logo-light.svg" alt="Keeper" width="560" />
  </picture>
</p>

<p align="center">
  <a href="./README.md">English</a> ｜ <a href="./README.zh.md"><strong>简体中文</strong></a>
</p>

<h1 align="center">CPA Usage Keeper</h1>

<p align="center">万千流转，皆有迹可循。</p>

<p align="center">
  <a href="https://github.com/Willxup/cpa-usage-keeper/releases/latest"><img src="https://img.shields.io/github/v/release/Willxup/cpa-usage-keeper?style=flat-square" alt="最新版本" /></a>
  <a href="https://github.com/Willxup/cpa-usage-keeper/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/Willxup/cpa-usage-keeper/ci.yml?branch=main&amp;style=flat-square&amp;label=CI" alt="CI 状态" /></a>
  <a href="https://github.com/Willxup/cpa-usage-keeper/pkgs/container/cpa-usage-keeper"><img src="https://img.shields.io/badge/Docker-GHCR-2496ED?style=flat-square&amp;logo=docker&amp;logoColor=white" alt="GHCR Docker 镜像" /></a>
  <a href="https://github.com/Willxup/homebrew-cpa-usage-keeper"><img src="https://img.shields.io/badge/Homebrew-supported-FBB040?style=flat-square&amp;logo=homebrew&amp;logoColor=black" alt="支持 Homebrew" /></a>
  <a href="https://github.com/Willxup/cpa-usage-keeper/releases/latest"><img src="https://img.shields.io/badge/Linux-FCC624?style=flat-square&amp;logo=linux&amp;logoColor=black" alt="支持 Linux" /></a>
  <a href="https://github.com/Willxup/cpa-usage-keeper/releases/latest"><img src="https://img.shields.io/badge/macOS-A2AAAD?style=flat-square&amp;logo=apple&amp;logoColor=black" alt="支持 macOS" /></a>
  <a href="https://github.com/Willxup/cpa-usage-keeper/releases/latest"><img src="https://img.shields.io/badge/Windows-0078D4?style=flat-square&amp;logo=data:image/svg%2Bxml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCAyNCI+PHBhdGggZmlsbD0iI2ZmZiIgZD0iTTIgMy41IDExIDJ2OUgyem0xMC0xLjdMMjIgLjNWMTFIMTJ6TTIgMTJoOXY5TDIgMTkuNXptMTAgMGgxMHYxMC43bC0xMC0xLjV6Ii8+PC9zdmc%2B" alt="支持 Windows" /></a>
  <a href="./LICENSE"><img src="https://img.shields.io/github/license/Willxup/cpa-usage-keeper?style=flat-square" alt="MIT License" /></a>
</p>

为 [CLIProxyAPI（CPA）](https://github.com/router-for-me/CLIProxyAPI) 保存用量历史，看清模型花费、请求表现和凭证限额。CPA Usage Keeper 将总览、实时诊断、请求明细与限额历史集中在一个独立部署的面板中。

**[已有 CPA，部署 Keeper](#keeper-only) · [首次部署 CPA + Keeper](#cpa--keeper) · [查看配置](#配置)**

## 界面预览

<table>
  <tr>
    <td width="50%" align="center">
      <strong>总览</strong>
      <picture>
        <source media="(prefers-color-scheme: dark)" srcset="./assets/screenshots/overview-dark.png" />
        <source media="(prefers-color-scheme: light)" srcset="./assets/screenshots/overview-white.png" />
        <img src="./assets/screenshots/overview-white.png" alt="CPA Usage Keeper 总览" width="100%" />
      </picture>
    </td>
    <td width="50%" align="center">
      <strong>实时诊断</strong>
      <picture>
        <source media="(prefers-color-scheme: dark)" srcset="./assets/screenshots/realtime-dark.png" />
        <source media="(prefers-color-scheme: light)" srcset="./assets/screenshots/realtime-white.png" />
        <img src="./assets/screenshots/realtime-white.png" alt="CPA Usage Keeper 实时诊断" width="100%" />
      </picture>
    </td>
  </tr>
  <tr>
    <td width="50%" align="center">
      <strong>用量分析</strong>
      <picture>
        <source media="(prefers-color-scheme: dark)" srcset="./assets/screenshots/analysis-dark.png" />
        <source media="(prefers-color-scheme: light)" srcset="./assets/screenshots/analysis-white.png" />
        <img src="./assets/screenshots/analysis-white.png" alt="CPA Usage Keeper 用量分析" width="100%" />
      </picture>
    </td>
    <td width="50%" align="center">
      <strong>请求事件</strong>
      <picture>
        <source media="(prefers-color-scheme: dark)" srcset="./assets/screenshots/request-events-dark.png" />
        <source media="(prefers-color-scheme: light)" srcset="./assets/screenshots/request-events-white.png" />
        <img src="./assets/screenshots/request-events-white.png" alt="CPA Usage Keeper 请求事件" width="100%" />
      </picture>
    </td>
  </tr>
  <tr>
    <td width="50%" align="center">
      <strong>凭证与限额</strong>
      <picture>
        <source media="(prefers-color-scheme: dark)" srcset="./assets/screenshots/credentials-dark.png" />
        <source media="(prefers-color-scheme: light)" srcset="./assets/screenshots/credentials-white.png" />
        <img src="./assets/screenshots/credentials-white.png" alt="CPA Usage Keeper 凭证与限额" width="100%" />
      </picture>
    </td>
    <td width="50%" align="center">
      <strong>限额历史</strong>
      <picture>
        <source media="(prefers-color-scheme: dark)" srcset="./assets/screenshots/quota-history-dark.png" />
        <source media="(prefers-color-scheme: light)" srcset="./assets/screenshots/quota-history-white.png" />
        <img src="./assets/screenshots/quota-history-white.png" alt="CPA Usage Keeper 限额历史" width="100%" />
      </picture>
    </td>
  </tr>
</table>

## 功能特性

- **保留历史**：持续保存 CPA 用量及执行、追踪元数据到 SQLite，并支持定时备份。
- **看清花费**：按模型、API Key 和提供商分析用量、缓存及估算成本；只有总 Token 数的报告会原样保留，不臆造可计费的输入、输出明细。
- **定位问题**：查看和导出请求明细，区分流式与非流式响应、复制原始 User-Agent，并通过成功率、首字延迟（TTFT）和总耗时分析请求表现。
- **掌握额度**：查看凭证健康与剩余限额，支持限额刷新、优先级编辑和 Codex 限额历史。
- **按需分享**：为单个 CPA API Key 提供独立的只读用量视图。

此外，还支持通过 CPA 插件嵌入 CPAMC。可使用 Docker Compose、Homebrew 或二进制部署，登录保护默认开启。

## 赞助与特别感谢

- 感谢 [CLIProxyAPI（CPA）](https://github.com/router-for-me/CLIProxyAPI) 提供本项目所依赖的上游 CPA 基础与数据来源。
- 感谢 [@YouShouldBetOnMe](https://github.com/YouShouldBetOnMe) 对 CPA Usage Keeper 的支持。
- 感谢 CPA 讨论组（QQ群组）的讨论与反馈。

## 快速开始

> Keeper 支持 CPA v8.0.12+。请开启 CPA 使用统计，将 `observability.usage.usage-statistics-enabled` 设为 `true`。

> 同一 CPA 接入多个 usage 采集服务时，请确保均使用订阅模式，否则可能导致收数中断或数据不完整。

Docker Compose 是推荐部署方式：首次部署可同时运行 CPA + Keeper，已有 CPA 时则使用 Keeper-only Compose。

使用 Docker 部署只需准备 Docker 和 Docker Compose，无需安装 Go、Node.js 或编译源码。已有 CPA 时，先准备 CPA 地址、管理密钥和一个 Keeper 登录密码。

| 场景 | 推荐方式 | 架构 |
| --- | --- | --- |
| 首次部署 CPA + Keeper | [Docker Compose：CPA + Keeper](#cpa--keeper) | `linux/amd64`、`linux/arm64` |
| 已有 CPA | [Docker Compose：仅 Keeper](#keeper-only) | `linux/amd64`、`linux/arm64` |
| 已有 CPA，偏好 Docker CLI | [Docker](#dockercpa-已在宿主机运行) | `linux/amd64`、`linux/arm64` |
| macOS | [Homebrew](#macos-homebrew) | `amd64`、`arm64` |
| Linux 不使用容器 | [Linux 二进制](#linux-二进制) | `amd64`、`arm64` |
| Windows | [Windows Binary](#windows-binary) | `amd64`、`arm64` |

登录保护默认启用。启动 Keeper 前请配置 `LOGIN_PASSWORD`；只有部署环境已可靠隔离访问时，才显式设置 `AUTH_ENABLED=false`。

### 本 fork 的 v2 升级说明

- 保持 glebarez/modernc SQLite、`CGO_ENABLED=0` 和六平台交叉编译；统一 `release.yml` 使用预编译二进制组装双架构运行时镜像，健康检查使用 `/readyz`。
- v2.0.1 接入 Credentials 路由与可选 OIDC 登录，默认不启用 OIDC。发布工具链使用 Go 1.26.10 和 Node 24.22.1；保留凭证时间范围/API Key 筛选及单凭证限额操作的多语言说明。
- v1 数据首次启动时先生成并校验迁移备份，再回填持久化费用。升级前另存一份完整备份；回退 v1 必须从该备份复制到独立工作目录，不能直接打开已迁移数据库。
- 价格管理切换为 v2 的编辑、拉取后审核应用和显式历史重算，旧一键同步入口不再保留。保存价格不会改变历史费用；保留 LiteLLM 名称规范化和已配置但不活跃模型的同步支持。
- 凭证汇总继续支持时间范围和 API Key 筛选，费用来自持久化结果；重算期间遵守 `costs_busy`。移动端请求卡片、客户端元数据及强制 ingest 模式继续保留。
- 排行功能及 `API_KEY_VIEWER_LOCAL_RANKING_ENABLED` 已移除；纯 Go benchmark 不提供原 CGO 原生 SQL trace，历史 benchmark 报告仍保留原测试环境信息。

## Benchmark

`linux/amd64` 生产型容量测试覆盖持续 ingestion、Dashboard 延迟、CPU 利用率和 Keeper cgroup 峰值内存，完整结果见 [容量 Benchmark 报告](./internal/benchmark/reports/capacity/README.zh.md)。

<details>
<summary>开发者参考：项目结构、本地运行与测试</summary>

## 项目结构

```text
cmd/server/              应用入口
internal/api/            HTTP 路由与处理器
internal/app/            应用装配与启动
internal/auth/           Session 与访问控制
internal/poller/         CPA 用量与配置同步
internal/repository/     SQLite 持久化与聚合
internal/service/        用量、定价与身份服务
internal/quota/          Provider 限额刷新与巡检
internal/benchmark/      容量套件、报告、manifest 与历史 Go microbenchmark
deploy/                  部署模板
web/                     React + TypeScript 前端
```

## 本地开发

### 前置依赖

- Go 1.26.10+
- Node.js 24.22.1+
- npm
- 一个可用的 [CLIProxyAPI（CPA）](https://github.com/router-for-me/CLIProxyAPI) 实例

使用 nvm 时，在仓库根目录运行 `nvm install` 和 `nvm use`，即可选择 `.nvmrc` 中指定的版本。

### 本地运行

1. 将 `.env.example` 复制为 `.env`，至少设置 `CPA_BASE_URL`、`CPA_MANAGEMENT_KEY` 和私有的 `LOGIN_PASSWORD`。

```bash
cp .env.example .env
vim .env
```

2. 启动后端。

```bash
go run ./cmd/server/main.go
```

3. 在另一个终端安装前端依赖并启动开发服务器。

```bash
npm --prefix ./web ci
npm --prefix ./web run dev -- --host 127.0.0.1
```

打开 `http://127.0.0.1:5173`。前端默认将 `/api` 代理到 `http://127.0.0.1:8080`；后端使用其它端口时可通过 `VITE_API_PROXY_TARGET` 覆盖。

### 测试

运行完整验证：

```bash
make verify
```

也可以分别运行：

```bash
go test ./cmd/... ./internal/...
npm --prefix ./web run test
npm --prefix ./web run lint
npm --prefix ./web run typecheck
npm --prefix ./web run build
```

### 图表时间处理

- 使用响应/配置时区。自然日使用 `internal/timeutil.CivilDate` 和 `DayBounds`，滚动窗口使用真实时长；自然日不一定是24小时。
- 通过 repository 的桶读取适配方法查询现有增量表。Overview 与 Latency 的小时锚点不同；Activity 保留既有364格及存量结束边界。查询修复不得改变写入桶或重切已有聚合。
- 原始事件时间条件与真实时间过滤复用 `internal/repository/usage_event_time_read.go`。普通查询使用有界分段，近期缓存启动/重载使用开放上界读取；字段投影与生命周期逻辑保留在调用方。
- 桶身份使用真实时刻或完整日期，不使用格式化钟面标签。时间戳按真实时间排序，日桶速率使用完整自然日的实际分钟数。
- 复用 `web/src/utils/usage/timeLabels.ts` 获取时区日期部分、区分回拨时的重复钟面。保留各图表原有语言、精度、fallback和补点规则，仅对歧义钟面补offset。
- 同时验证上海普通日期和DST跳时/回拨，覆盖浏览器与响应时区不同的情况；保留现有图表样式和生命周期行为。

</details>

## 部署方式

启动后访问 `http://服务器地址:8080`（本机部署可用 `http://127.0.0.1:8080`），使用配置的 Keeper 登录密码登录。修改端口或配置 HTTPS、子路径时，请使用对应地址。

### Docker Compose（推荐）

Docker Compose 同时推荐用于 CPA + Keeper 联合部署和 Keeper 单独部署。

#### CPA + Keeper

**1. 准备配置文件**

先在部署目录中从 [CPA 官方仓库](https://github.com/router-for-me/CLIProxyAPI) 下载配置示例，保存为 `./cpa/config.yaml`：

```bash
mkdir -p cpa/auths cpa/logs keeper
curl -fL https://raw.githubusercontent.com/router-for-me/CLIProxyAPI/main/config.example.yaml \
  -o cpa/config.yaml
```

在同一目录下载 [CPA + Keeper 联合部署模板](./deploy/docker-compose.full.example.yml)：

```bash
curl -fL https://raw.githubusercontent.com/Willxup/cpa-usage-keeper/main/deploy/docker-compose.full.example.yml \
  -o docker-compose.yml
```

以上下载命令用于首次部署；已有配置时请直接编辑，避免覆盖。

**2. 填写 CPA 和 Keeper 配置**

按下表编辑 `cpa/config.yaml` 中的已有配置项，保留 YAML 层级与缩进；表中的点号表示嵌套路径，不是要新增的 YAML 键名：

| 配置项 | 设置说明 |
| --- | --- |
| `management.allow-remote` | 改为 `true`，允许 Keeper 从另一个容器访问 CPA 管理功能。 |
| `management.secret-key` | 设置私有的管理密钥，并在 `docker-compose.yml` 的 `CPA_MANAGEMENT_KEY` 中填写同一个原始密钥。 |
| `observability.usage.usage-statistics-enabled` | 改为 `true`，开启使用统计。 |
| `access.api-keys` | 将示例密钥替换为自己的客户端调用密钥；这不是 CPA 管理密钥或 Keeper 登录密码。 |

保留 `server.host` 为空字符串、`server.port` 为 `8317`、`oauth.auth-dir` 为 `"~/.cli-proxy-api"`，以匹配模板中的容器网络及目录挂载。

编辑下载的 `docker-compose.yml`，在 Keeper 的 `environment` 中填写两项：

- `CPA_MANAGEMENT_KEY`：填写 CPA 中设置的同一个原始管理密钥。
- `LOGIN_PASSWORD`：将空字符串 `""` 替换为你自己的 Keeper 登录密码。

也可将配置写入 `./keeper/.env`，模板会通过可选的 `env_file` 读取；文件不存在时不影响 Compose 启动。同名变量以 `environment` 为准，如改用文件中的值，请删除 `environment` 中对应项，包括空值占位。

**3. 启动并访问**

```bash
docker compose up -d
```

访问 `http://服务器地址:8080`，使用刚设置的 Keeper 登录密码登录。停止服务时执行 `docker compose down`。

首次部署还需在 CPA 中添加模型凭证，调用模型后才会产生使用记录。

CPA 数据保存在 `./cpa`，Keeper 数据保存在 `./keeper`。

#### Keeper Only

**1. 准备配置文件**

CPA 已经部署好时，在新的部署目录中下载 Keeper-only Compose 模板和环境配置：

```bash
curl -fL https://raw.githubusercontent.com/Willxup/cpa-usage-keeper/main/deploy/docker-compose.example.yml \
  -o docker-compose.yml
curl -fL https://raw.githubusercontent.com/Willxup/cpa-usage-keeper/main/.env.example -o .env
vim .env
```

已有部署请直接编辑现有文件，避免覆盖配置。

**2. 填写连接信息和登录密码**

CPA 运行在 Docker 宿主机上时，可从以下配置开始：

```env
CPA_BASE_URL=http://host.docker.internal:8317
CPA_MANAGEMENT_KEY=replace-with-your-management-key
AUTH_ENABLED=true
LOGIN_PASSWORD=
```

启动容器前请设置私有的 `LOGIN_PASSWORD`。

无论 CPA 位于 Docker 宿主机还是其他主机，都需允许远程管理，并监听 Keeper 容器可访问的地址；只监听 `127.0.0.1` 时容器无法连接。设置 `management.allow-remote: true`，并检查 `server.host`。

其它网络环境请将 `CPA_BASE_URL` 改为容器可访问的 CPA 地址。只有 Redis/RESP 地址与自动推导的地址不同时，才需要设置 `REDIS_QUEUE_ADDR`。

**3. 启动并访问**

```bash
docker compose up -d
```

访问 `http://服务器地址:8080`，使用刚设置的 Keeper 登录密码登录。停止服务时执行 `docker compose down`。

模板默认将 Keeper 数据保存在 `./data`。

#### 查看日志与更新

两种 Compose 部署均可在部署目录中查看 Keeper 日志：

```bash
docker compose logs --tail=100 -f cpa-usage-keeper
```

更新 Keeper 时，保留配置和数据目录，执行：

```bash
docker compose pull cpa-usage-keeper
docker compose up -d cpa-usage-keeper
```

#### 启动后没有数据？

- 确认 CPA 使用统计已开启：`observability.usage.usage-statistics-enabled` 为 `true`。
- 确认 Keeper 能访问 `CPA_BASE_URL`，且 `CPA_MANAGEMENT_KEY` 与 CPA 管理密钥一致。
- 确认 CPA 已产生新的模型请求；仍没有数据时，用上面的日志命令检查连接或认证错误。

### Docker（CPA 已在宿主机运行）

偏好使用 `docker run` 时，复用上面 Keeper-only Compose 的 `.env` 配置：

```bash
docker run -d \
  --name cpa-usage-keeper \
  --add-host=host.docker.internal:host-gateway \
  -p 8080:8080 \
  -v "$(pwd)/keeper:/data" \
  --env-file .env \
  ghcr.io/willxup/cpa-usage-keeper:latest
```

### macOS Homebrew

Homebrew 是 macOS 推荐安装方式：

```bash
brew tap Willxup/cpa-usage-keeper
brew install cpa-usage-keeper
```

设置 `CPA_BASE_URL`、`CPA_MANAGEMENT_KEY` 和私有的 `LOGIN_PASSWORD`，然后启动服务：

```bash
vim "$(brew --prefix)/etc/cpa-usage-keeper.env"
brew services start cpa-usage-keeper
```

升级和服务管理命令：

```bash
brew services list
brew services restart cpa-usage-keeper
brew update
brew upgrade cpa-usage-keeper
```

数据保存在 `$(brew --prefix)/var/cpa-usage-keeper`，日志写入 `$(brew --prefix)/var/log/`。

### Linux 二进制

从 [Releases](https://github.com/Willxup/cpa-usage-keeper/releases/latest) 下载 `linux_amd64` 或 `linux_arm64` 压缩包，然后解压并运行：

```bash
mkdir -p cpa-usage-keeper
tar -xzf ./cpa-usage-keeper_*_linux_*.tar.gz -C cpa-usage-keeper --strip-components=1
cd cpa-usage-keeper
cp .env.example .env
vim .env
./cpa-usage-keeper
```

#### systemd

Linux 压缩包内置 service 模板。请在解压后的目录中运行：

```bash
sudo cp cpa-usage-keeper.service /etc/systemd/system/cpa-usage-keeper.service
sudo sed -i "s|__CPA_USAGE_KEEPER_DIR__|$(pwd)|g" /etc/systemd/system/cpa-usage-keeper.service
sudo systemctl daemon-reload
sudo systemctl enable --now cpa-usage-keeper
```

```bash
sudo systemctl status cpa-usage-keeper
sudo journalctl -u cpa-usage-keeper -f
sudo systemctl restart cpa-usage-keeper
```

### 命令行参数

二进制支持以下可选启动参数：

```bash
cpa-usage-keeper --env /path/to/keeper.env # 指定环境配置文件。
cpa-usage-keeper --host 127.0.0.1 # 仅为当前进程覆盖 APP_HOST。
cpa-usage-keeper -v               # 输出构建版本并退出；也支持 --version。
```

### Windows Binary

从 [Releases](https://github.com/Willxup/cpa-usage-keeper/releases/latest) 下载 `windows_amd64` 或 `windows_arm64` ZIP 并解压。在 PowerShell 中进入解压目录后运行：

```powershell
Copy-Item .env.example .env
notepad .env
.\cpa-usage-keeper.exe
```

启动前请设置 `CPA_BASE_URL`、`CPA_MANAGEMENT_KEY` 和私有的 `LOGIN_PASSWORD`。认证默认启用；只有隔离部署才显式设置 `AUTH_ENABLED=false`。

## 配置

复制配置模板：

```bash
cp .env.example .env
```

首次部署先填写 CPA 地址、CPA 管理密钥和 Keeper 登录密码，其余配置通常保持默认。需要域名访问、HTTPS 或子路径时，再查看对应章节。

### 最小必填

| 变量 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `CPA_BASE_URL` | 是 | - | Keeper 服务端访问 CPA 的地址。Docker Compose 内通常是 `http://cli-proxy-api:8317`，可以是内网地址或容器服务名 |
| `CPA_MANAGEMENT_KEY` | 是 | - | CPA management key，用于读取 CPA 管理接口数据 |

### Web 访问与反代

| 变量 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `APP_HOST` | 否 | 所有接口 | Keeper HTTP 监听主机；原生部署仅允许本机访问时可设为 `127.0.0.1` |
| `APP_PORT` | 否 | `8080` | Keeper HTTP 监听端口 |
| `APP_BASE_PATH` | 否 | 根路径 | Keeper 子路径部署前缀，例如 `/keeper`；留空表示部署在 `/` |
| `CPA_PUBLIC_URL` | 否 | 当前浏览器同源根路径 | 浏览器访问 CPA 的公开地址，用于“返回 CPA”跳转和 CPAMC frame 信任来源 |
| `FRAME_ANCESTOR_ORIGINS` | 否 | 空 | 额外信任的 `frame-ancestors` 来源，逗号分隔，每项必须是带 host 的 `http://` 或 `https://` URL |
| `TRUSTED_PROXY_CIDRS` | 否 | 仅本机 loopback | 允许提供 `X-Forwarded-For` 的额外反向代理 CIDR，多个值用逗号分隔 |

- 启动参数 `--host` 的优先级高于 `APP_HOST`。两者都未设置时，Keeper 保持现有行为，监听所有可用网络接口。
- Docker/Compose 请保持 `APP_HOST` 为空；如需仅允许 Docker 宿主机访问，请将端口发布为 `127.0.0.1:8080:8080`。
- `APP_BASE_PATH` 必须为空或以 `/` 开头；`/cpa/` 会规范为 `/cpa`。
- `CPA_BASE_URL` 是服务端访问 CPA 的地址，可以使用内网地址或 Docker 服务名。
- `CPA_PUBLIC_URL` 控制浏览器跳转和跨域 CPAMC frame 信任。同源且 CPA 位于 `/management.html` 时可留空；域名、端口或路径不同时应设置公开 CPA 地址。
- Keeper 只信任本机 loopback 和 `TRUSTED_PROXY_CIDRS` 提供的 `X-Forwarded-For`；直连客户端不能通过该请求头切换登录限流来源。只配置实际代理地址或网段，全网 CIDR 会被拒绝。

`CPA_PUBLIC_URL` 可填写域名、带协议的完整地址或相对路径，例如 `https://cpa.example.com`、`https://cpa.example.com/cpa/` 或 `/cpa/`。前端会自动追加 `management.html`，并兼容末尾已有 `/` 或已经填写到 `management.html` 的情况。未配置时，“返回 CPA”默认跳转到当前浏览器同源根路径下的 `/management.html`；如果 CPA 和 Keeper 的外部域名、端口或路径不一致，请显式设置 `CPA_PUBLIC_URL`。

用于 CPAMC frame 信任时，`CPA_PUBLIC_URL` 必须是带 host 的显式 `http://` 或 `https://` URL。相对路径只影响同源“返回 CPA”跳转，不会增加外部 `frame-ancestors` 来源。

如果需要在 `CPA_PUBLIC_URL` 之外信任其它嵌入方（例如独立部署的管理面板域名），使用 `FRAME_ANCESTOR_ORIGINS` 追加来源，逗号分隔，例如 `FRAME_ANCESTOR_ORIGINS=https://cpamc.example.com,http://10.0.0.2:18317`。每项会归一化为 `scheme://host` 并与 `CPA_PUBLIC_URL` 的 origin 合并去重；非法项会在启动时报错。

`CPA_BASE_URL` 只用于服务端访问 CPA，可以是 Docker 内部服务名或内网地址；不会用于浏览器跳转或 frame 信任判断。

### 登录保护

| 变量 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `AUTH_ENABLED` | 否 | `true` | 是否启用登录保护 |
| `LOGIN_PASSWORD` | 鉴权启用时必填 | - | 登录密码 |
| `CPA_REQUEST_LOG_ACCESS_ENABLED` | 否 | `false` | 允许管理员通过 Keeper 查看和下载 CPA 请求日志；需要 CPA 中存在对应日志，内容可能包含请求或响应数据 |
| `AUTH_SESSION_TTL` | 否 | `168h` | 登录 session 有效时长 |

### OIDC 管理员登录

Keeper 可通过一个 OpenID Connect 身份平台登录管理员。在平台中注册带客户端密钥的服务器端客户端，启用 Authorization Code Flow 和 PKCE S256，并精确注册公开回调地址：

```text
https://keeper.example.com/<APP_BASE_PATH>/api/v1/auth/oidc/callback
```

根路径部署省略 `<APP_BASE_PATH>/`。仍须保留 `AUTH_ENABLED=true` 和私有的 `LOGIN_PASSWORD`；身份平台暂时不可用时，密码登录继续可用。

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `AUTH_OIDC_ENABLED` | `false` | 在独立页面的管理员登录页签启用 OIDC |
| `AUTH_OIDC_PROMPT_LOGIN` | `true` | 发起登录时传递 `prompt=login` 要求重新认证；设为 `false` 时不传此参数，允许复用平台已有登录状态 |
| `AUTH_OIDC_ISSUER` | 空 | 精确的 HTTP/HTTPS Issuer，包含平台租户路径 |
| `AUTH_OIDC_CLIENT_ID` | 空 | 已注册的服务器端客户端 ID |
| `AUTH_OIDC_CLIENT_SECRET` | 空 | 已注册的客户端密钥 |
| `AUTH_OIDC_REDIRECT_URL` | 空 | 精确的 Keeper 公开回调地址，包含部署子路径 |
| `AUTH_OIDC_ALLOWED_SUBJECTS` | `[]` | 可选的 JSON 字符串数组，精确匹配区分大小写的 `sub` |

例如 `AUTH_OIDC_ALLOWED_SUBJECTS='["user-123","user,456"]'`。请使用平台实际签发给此客户端的 `sub`，不要替换成邮箱、显示名或猜测的平台用户 ID。缺省或空数组将应用准入交给身份平台：**通过此客户端认证的所有用户都会成为 Keeper 管理员**，请在平台中配置该应用的访问策略。邮箱和姓名仅用于显示。旧的 `AUTH_OIDC_ALLOWED_USERS` 非空时会阻止启动并提示迁移；删除旧配置前，须换成实际 subject，或在身份平台实施相应访问限制。

每次新发起 OIDC 登录时，默认通过 `prompt=login` 要求重新认证，具体认证方式由身份平台决定。设置 `AUTH_OIDC_PROMPT_LOGIN=false` 可允许复用平台现有登录状态。此设置不会撤销已建立的 Keeper 会话，也不会退出身份平台。

Keeper 使用标准 Discovery、JWKS、`openid profile email`、有签名的 ID Token、nonce、浏览器绑定及 PKCE S256。客户端密钥认证自动处理 `client_secret_basic` 和 `client_secret_post`。元数据端点可位于不同主机，并使用显式 HTTP/HTTPS 地址和已有查询参数。公共客户端、`form_post`、JWE、JWT Client Authentication、多 Issuer 选择和全局单点注销不属于本功能范围。

推荐 Issuer、元数据端点和 Keeper 均使用 HTTPS。明确配置的 HTTP 地址直接生效，无需额外开关，也不限于 localhost；HTTP Discovery/JWKS 无法保护身份信任起点，HTTP Keeper Cookie 没有传输加密。HTTPS 失败不会自动降级 HTTP 或跳过证书验证。内部 CA 需要加入服务使用的受信任证书链。配置或元数据使用 HTTP 时会记录提示；HTTP 支持是对 Discovery 的 HTTPS 要求的兼容扩展，不会让明文链路获得传输安全。

Issuer 使用平台公布的身份地址，须与 Discovery 和 ID Token 的 `iss` 精确一致。Keeper 服务端需要能访问 Discovery、Token 和 JWKS 端点，浏览器需要能访问授权端点；这些端点不要求同一主机。除非平台本身公布的身份地址就是容器内部地址，否则不能将 Issuer 随意替换为内部地址。

请从与回调相同的公开 origin 打开 Keeper。HTTPS 反向代理应保留公开 `Host` 和 `X-Forwarded-Proto`（或提供 `X-Forwarded-Host`），并在 `TRUSTED_PROXY_CIDRS` 中配置实际代理网段。API Key 页签和 CPAMC 嵌入模式不显示 OIDC。登录成功会恢复合法的站内页面及查询参数。未完成登录保存在当前进程内，五分钟过期；重启中断的登录可以重新点击按钮发起。请在代理访问日志中过滤回调查询参数，其中包含短期授权码和 state。

取消授权和身份平台故障不计入 OIDC 的来源认证失败额度；现有全局登录请求频率限制仍然生效。

登录后的会话沿用 `AUTH_SESSION_TTL`，在会话管理中显示 **OIDC 登录**。退出只撤销 Keeper 本地会话；当 `AUTH_OIDC_PROMPT_LOGIN=false` 时，再次登录仍可能由身份平台自动认证。关闭 OIDC、切换 Issuer/Client ID 或收紧 subject 列表后，配置随重启生效，在业务就绪前永久撤销受影响会话，无需旧浏览器再次访问或打开会话管理页；仅更换客户端密钥不会撤销现有会话。平台侧删号或收紧访问权限不会自动撤销 Keeper 已有会话，需要在会话管理中撤销或等待 TTL 到期。回退到不支持 OIDC 的 Keeper 版本前，先用新版本撤销所有 OIDC 会话，再停止 Keeper 并替换程序；新增可空数据库列可以保留。

### 时区与请求行为

| 变量 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `TZ` | 否 | `Asia/Shanghai` | 统计和展示使用的时区；Today、按天统计、页面时间、日志时间和每日清理时间都会按这个时区计算 |
| `REQUEST_TIMEOUT` | 否 | `30s` | 请求 CPA HTTP 接口和 Redis 队列的超时时间 |
| `TLS_SKIP_VERIFY` | 否 | `false` | 跳过 CPA HTTPS 和 Redis 队列 TLS 的证书验证；仅在使用自签名证书时启用 |

### Credentials 限额刷新

Credentials 定时限额刷新在 Credentials 巡检弹窗的小齿轮中配置。设置保存在本地 SQLite，不依赖页面保持打开。

| 变量 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `QUOTA_REFRESH_WORKER_LIMIT` | 否 | `10` | 手动刷新和定时刷新共用的 Credentials 限额刷新队列最大并发数，最大 `100` |
| `QUOTA_UPSTREAM_RESPONSES_ENABLED` | 否 | `false` | 缓存每个凭证最近一次限额查询的原始上游响应，并通过 quota task/cache API 返回，供浏览器 Network 面板排障；响应可能包含账号数据 |

### Redis 队列高级配置

| 变量 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `REDIS_QUEUE_ADDR` | 否 | 从 `CPA_BASE_URL` 推导 | 显式设置时优先使用该地址；留空时沿用 `CPA_BASE_URL` 的主机和显式端口，未指定端口时使用 `8317`。独立地址填写 `host:port` |
| `REDIS_QUEUE_TLS` | 否 | `false` | 设为 `true` 强制启用 Redis 队列 TLS；未指定队列地址时，默认跟随 `CPA_BASE_URL` 的协议。`false` 不会覆盖自动推导的 TLS 设置。 |
| `REDIS_QUEUE_BATCH_SIZE` | 否 | `10000` | 每次最多拉取的队列记录数 |
| `REDIS_QUEUE_IDLE_INTERVAL` | 否 | `1s` | 队列为空时的检查间隔 |
| `REDIS_INGEST_MODE` | 否 | `auto` | 远端 ingest 长期模式：`auto`、`subscribe`、`redis_pull`、`http_pull`。固定模式会跳过其他模式的启动探测并保持所选长期模式；`subscribe` 与 `redis_pull` 仍保留故障期间的临时 fallback 和恢复探测 |
| `REDIS_INGEST_RECOVERY_INTERVAL` | 否 | `30s` | `subscribe` 断线重连和 `redis_pull` 恢复探测的间隔；多消费端共用 CPA 时建议调小 |

### 存储、日志与备份

| 变量 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `WORK_DIR` | 否 | `./data` | 应用工作目录；数据库、日志和备份默认分别写入 `app.db`、`logs/`、`backups/` |
| `LOG_LEVEL` | 否 | `info` | 日志级别 |
| `LOG_FILE_ENABLED` | 否 | `true` | 是否写入持久化日志文件 |
| `LOG_RETENTION_DAYS` | 否 | `7` | 综合日志保留历史天数，并额外保留当天；`0` 表示不自动清理。仅错误日志固定保留历史 30 天及当天 |
| `BACKUP_ENABLED` | 否 | `true` | 是否启用定时数据库备份 |
| `BACKUP_INTERVAL` | 否 | `24h` | 数据库备份间隔 |
| `BACKUP_RETENTION_DAYS` | 否 | `7` | 备份保留天数 |
| `USAGE_RAW_RETENTION_DAYS` | 否 | `0` | 原始请求总保留天数，仅清理归档表；`0` 永久保留，`>=90` 生效，负数及 `1～89` 按 `0` 处理并记录 warning，不影响启动 |

Keeper 每天按配置时区在 04:30 自动归档超过 90 个本地自然日的原始请求记录。归档默认永久保留。设置 `USAGE_RAW_RETENTION_DAYS>=90` 后，每日归档完成后会分批删除超过总保留天数的归档记录，期限按请求发生时间及配置时区的自然日计算；热表的 90 天窗口不变。常规页面不直接查询归档明细；历史汇总和备份继续采用各自的保留策略。删除的原始记录无法用于后续历史重算；SQLite 文件不一定立即缩小，空闲空间可复用，并由现有条件式整理回收。

启用文件日志后，`cpa-usage-keeper-YYYY-MM-DD.log` 会记录所有已输出级别；error、fatal 和 panic 级别还会同时写入 `cpa-usage-keeper-error-YYYY-MM-DD.log`，该文件固定保留历史 30 个本地自然日及当天。

### v1 升级 v2 失败后的回退

Keeper 会在迁移 v1 数据前备份数据库。如果迁移失败且备份验证通过，日志会输出英文回退说明及备份路径 `backup_path`。

1. 停止 Keeper v2，保留故障数据库及其可能存在的 `-wal`、`-shm` 文件。
2. 将升级前备份复制为独立数据目录中的 `<新 WORK_DIR>/app.db`，即将备份副本重命名为 `app.db`。保留原备份，不要混入故障数据库的文件。
3. 安装 Keeper v1，将 `WORK_DIR` 设为该新目录。Docker 默认配置下，将新目录挂载到 `/data`，使恢复文件在容器内的路径为 `/data/app.db`。不要让 v1 直接打开迁移了一部分的数据库。

恢复后的数据回到备份时点，不包含此后收到的用量。如果没有可验证的备份，应先找到并核实一份升级前备份，再尝试回退。

### 内置 HTTPS

| 变量 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `TLS_ENABLED` | 否 | `false` | 是否让 Keeper 自己启用 HTTPS/TLS |
| `TLS_CERT_FILE` | 启用 TLS 时必填 | - | HTTPS 证书文件路径 |
| `TLS_KEY_FILE` | 启用 TLS 时必填 | - | HTTPS 私钥文件路径 |

通常建议在 nginx、Caddy 等反向代理层处理 HTTPS。只有需要 Keeper 进程直接提供 HTTPS 时，才设置 `TLS_ENABLED=true`，并填写 `TLS_CERT_FILE` 和 `TLS_KEY_FILE`；相对路径会按 `.env` 所在目录解析。

安全与数据说明：

- SQLite 数据库备份会保存应用数据库中的原始数据，备份文件不做加密。
- 面向浏览器的 API 会对 key-like source/lookup 字段做脱敏或稳定公开标识映射，但不会修改数据库原始值。
- 认证默认启用。若显式关闭，请在部署边界限制 Keeper 访问；公网访问应在反向代理层启用 HTTPS。
- 登录 session hash 存在 SQLite 中，服务重启后仍会保持有效，直到用户退出登录或超过 `AUTH_SESSION_TTL`。
- CPAMC 嵌入模式使用独立的 embed session。Keeper 会先尝试 `HttpOnly` 的 `cpa_usage_keeper_embed_session` cookie；如果浏览器无法保存嵌入 cookie，则回退到仅 embed 模式使用的请求头。普通 Dashboard session 仍保持 `SameSite=Lax`，不会被嵌入视图复用。
- 同源 CPAMC 嵌入使用默认的 `frame-ancestors 'self'` 即可。跨源 CPAMC 嵌入时，请将 `CPA_PUBLIC_URL` 设置为公开的 CPA/CPAMC origin，或通过 `FRAME_ANCESTOR_ORIGINS` 追加多个信任来源；Keeper 不会使用 `CPA_BASE_URL` 作为 `frame-ancestors` 来源。
- 跨站点嵌入登录在 HTTPS 且浏览器支持第三方 cookie 或分区 cookie 时体验最好。如果 cookie 路径不可用，CPAMC 嵌入模式会回退到按标签页保存在浏览器 session storage 中的 header token。
- Redis inbox 原始消息会自动清理：成功数据保留到当天结束后清理，失败数据保留 7 天。

## Nginx 反向代理

部署到 `/cpa` 时设置 `APP_BASE_PATH=/cpa`，并在反向代理中保留该前缀：

```nginx
location /cpa/ {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
}
```

上面的本机 Nginx 配置无需额外设置 Keeper。若反向代理通过容器或其它主机访问 Keeper，或反向代理前面还有 Cloudflare 等 CDN，请加入准确的代理网段，例如 `TRUSTED_PROXY_CIDRS=172.18.0.0/16`。

CPA 与 Keeper 浏览器同源时，可以不设置 `CPA_PUBLIC_URL`，“返回 CPA”默认使用 `/management.html`。CPA 位于其它域名、端口或路径时，设置公开地址：

```env
CPA_PUBLIC_URL=https://cpa.example.com
```

## License

本项目基于 [MIT License](./LICENSE) 开源。

## Star History

<a href="https://star-history.dera.page/Willxup/cpa-usage-keeper">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://star-history.dera.page/svg?repos=Willxup/cpa-usage-keeper&theme=dark" />
   <source media="(prefers-color-scheme: light)" srcset="https://star-history.dera.page/svg?repos=Willxup/cpa-usage-keeper" />
   <img alt="Star History Chart" src="https://star-history.dera.page/svg?repos=Willxup/cpa-usage-keeper" />
 </picture>
</a>
