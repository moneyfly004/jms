# JMS 节点采集器

自动化采集 GitHub 上包含 `jmssub.net` / `jjsubmarines.com` 等关键词的代码资源，提取订阅链接，
解析节点并使用 **mihomo 内核**真实代理测速，最终只保存可用节点。

- **代理内核**： [mihomo](https://github.com/MetaCubeX/mihomo)（原 Clash.Meta），不再使用 sing-box
- **测速方式**： 生成最小 mihomo 配置 → 启动内核 → 通过 mixed-port 真实请求测速（不是纯 TCP 连通性）
- **自动化**： GitHub Actions 每天定时采集并自动提交 `nodes.txt`

## 📡 订阅地址

### JMS 节点订阅（nodes.txt）
```
https://gist.githubusercontent.com/moneyfly004/e2b2b5f89928dcb48a62d6394504a324/raw/nodes.txt
```

### 订阅链接节点订阅（sub.txt）
```
https://gist.githubusercontent.com/moneyfly004/fa0e0bfff22b50d9ca9d92b82ebe2f7c/raw/sub.txt
```
```
https://gist.githubusercontent.com/moneyfly1/73a34355ea99f43d02d7916771d336d5/raw/all.yaml
https://gist.githubusercontent.com/moneyfly1/73a34355ea99f43d02d7916771d336d5/raw/base64.txt
```

`nodes.txt` 的内容是 Base64 编码的节点链接（每行一个），可直接作为订阅地址使用。

## 📁 项目结构

```
.
├── main.go                 # 入口：搜索关键词 → 提取链接 → 解析节点 → 测速 → 落盘/Gist
├── node_parser.go          # 各类分享链接（ss/vmess/vless/trojan/hysteria2/tuic/...）解析器
├── mihomo.go               # mihomo 内核：配置生成、启动、代理测速
├── gist.go                 # 推送结果到 GitHub Gist
├── mihomo_test.go          # 测试：用真实 mihomo 内核校验生成的配置 + 端到端代理测试
├── links.txt / nodes.txt   # 采集结果（nodes.txt 为 Base64）
├── keywords.txt            # 搜索关键词，一行一个，`#` 开头为注释
├── mihomo/mihomo           # 内置的 Linux amd64 内核（GitHub Actions 使用）
├── scripts/                # 内核下载脚本 + 推送辅助脚本
└── .github/workflows/      # GitHub Actions 工作流
```

## 🚀 快速开始

### 1. 准备 Go 环境

需要 Go **1.24+**（`go.mod` 中声明）。

### 2. 准备 mihomo 内核

仓库里**内置了 GitHub Actions 真正使用的 Linux amd64 内核**：

```
mihomo/
├── mihomo       # Linux amd64（入库，GitHub Actions 使用）
└── mihomo.exe   # Windows amd64（不入库，仅本地调试用）
```

因此在 Linux 上可以直接运行；只有在 **Windows 本地调试**时才需要额外下载对应平台的内核：

```powershell
# Windows：下载 mihomo.exe 到 .\mihomo\
pwsh -File .\scripts\download-mihomo.ps1

# 已存在时跳过，需要升级/覆盖加 -Force
pwsh -File .\scripts\download-mihomo.ps1 -Force
```

```bash
# Linux / macOS：仓库里已有 Linux 版则会跳过，缺失或需要升级时才真正下载
chmod +x ./scripts/download-mihomo.sh
./scripts/download-mihomo.sh              # 默认 v1.19.31
./scripts/download-mihomo.sh v1.19.31     # 指定版本
./scripts/download-mihomo.sh --force      # 强制重新下载
```

内核定位规则：`MIHOMO_PATH` 环境变量 → `./mihomo/mihomo`（Windows 为 `mihomo.exe`）→ `PATH`。

> ⚠️ 内核与平台必须匹配：Windows 的 `mihomo.exe` 不能在 Linux 的 GitHub Actions 上运行，
> 反之亦然。工作流里用 `file ./mihomo/mihomo` + `mihomo -v` 做了显式校验。

### 3. 运行

```bash
go run .
```

只测试节点文件、不采集：

```bash
go run . test-nodes nodes-decoded.txt
```

### 4. 编译为可执行文件

```bash
go build -o jms-collector .
./jms-collector
```

## ⚙️ 环境变量

| 变量名 | 说明 | 必需 | 默认值 |
|--------|------|------|--------|
| `GITHUB_TOKEN` | GitHub Token（搜索代码、推送 Gist） | 否 | - |
| `GIST_ID` | GitHub Gist ID（设置后更新同一个 Gist） | 否 | - |
| `GIST_TOKEN` | 推送 Gist 用的 Token | 否 | 回退到 `GITHUB_TOKEN` |
| `MIHOMO_PATH` | mihomo 内核可执行文件路径 | 否 | 自动探测 `./mihomo/mihomo` |
| `MAX_CONCURRENCY` | 并发测速数量 | 否 | `30` |
| `TEST_TIMEOUT` | 单节点测速超时（秒） | 否 | `15` |
| `TEST_COUNT` | 每个节点重复测速次数（1~3） | 否 | `1` |
| `TEST_URL` | 自定义测速目标地址 | 否 | 内置多个地址轮询 |
| `TEST_SPEED` | 预留：是否测速带宽 | 否 | `false` |

## 🤖 GitHub Actions 自动化

### 1. 把改动推送到 GitHub

工作流文件位于仓库的 `.github/workflows/` 目录下，**必须提交到 GitHub 才会生效**：

| 文件 | 作用 |
|------|------|
| `.github/workflows/collect-nodes.yml` | 每天 UTC 02:00（北京 10:00）自动采集节点并提交 `nodes.txt` |
| `.github/workflows/build.yml` | 每次 push / PR 自动编译、静态检查、跑测试 |

本次迁移已经删除了仓库里的 sing-box 内核目录，请在提交时**一并提交这些删除**，否则 GitHub 上仍会保留旧内核。

如果本地目录没有 `.git`（例如是下载 ZIP 解压得到的），可以用仓库自带的脚本一步完成：

```powershell
pwsh -File .\scripts\push-to-github.ps1 -WhatIfOnly   # 先预览会做什么
pwsh -File .\scripts\push-to-github.ps1               # 真正推送
```

脚本会 `git fetch` 远程历史，再把本地改动挂到远程分支之上（不会丢失 GitHub 上的历史），
最后生成一个可 fast-forward 的提交并推送。

手动推送的命令：

```bash
git add -A
git commit -m "feat: 迁移到 mihomo 内核并添加 GitHub Actions 工作流"
git push origin main
```

> ⚠️ 如果 `.github/workflows/` 没有提交，仓库的 **Actions** 页面就是空的。
> 另外首次使用时需要在该页面点击 **Enable Actions** 手动启用。

### 2. 配置 Secrets

仓库 → **Settings** → **Secrets and variables** → **Actions** → **New repository secret**

| Secret 名称 | 必需 | 说明 |
|------------|------|------|
| `GH_PAT` | 可选（推荐） | 个人 PAT，用于代码搜索 / 推送 Gist。不配置时自动回退到 `GITHUB_TOKEN` |
| `GIST_ID` | 可选 | 已有的 Gist ID，配置后每次更新同一个 Gist，订阅地址保持不变 |
| `GIST_TOKEN` | 可选 | 推送 Gist 用的 Token，回退到 `GH_PAT` / `GITHUB_TOKEN` |
| `MAX_CONCURRENCY` | 可选 | 覆盖默认并发数 |
| `TEST_TIMEOUT` | 可选 | 覆盖默认测速超时 |
| `TEST_COUNT` | 可选 | 覆盖默认重复测速次数 |

**PAT 所需权限**（Settings → Developer settings → Personal access tokens）：

- `public_repo` — 搜索公开代码、推送代码
- `repo` — 如果仓库是私有的
- `gist` — 创建 / 更新 Gist

> `GITHUB_TOKEN` 是 Actions 自动提供的，**无需手动创建**，但它对代码搜索 API 的权限有限；
> 如果日志里出现搜索 403，请配置 `GH_PAT`。

### 3. 手动触发

**Actions** → **自动采集节点** → **Run workflow**，可临时指定并发数与超时。

### 4. 工作流做了什么

1. 检出代码，安装 Go（版本取自 `go.mod`）
2. 准备 **Linux amd64** 版 mihomo 内核（仓库里已有则直接用，缺失才在线下载）
3. 执行 `go run .`：搜索关键词 → 解析节点 → mihomo 真实代理测速
4. 把 `nodes.txt` / `links.txt` 上传为构建产物（保留 7 天）
5. 有变化时自动提交并推送到当前分支
6. 在运行摘要中输出关键词数、链接数与可用节点数

## 🔍 支持的节点协议

mihomo 测速支持下列协议，全部由 `mihomo -t` 在测试中校验过：

`ss`（含 `obfs` / `v2ray-plugin` / `shadow-tls` / `restls` 插件）、`ssr`、`vmess`、`vless`（含 REALITY）、
`trojan`、`hysteria`、`hysteria2`、`tuic`、`wireguard`、`http`/`https`、`socks5`、`anytls`

传输层支持 `tcp` / `ws` / `grpc` / `h2` / `http`。

不支持：`socks4`、`gost`（内核不支持，会被自动跳过）。

## 🧪 测试

```bash
# 需要先下载 mihomo 内核；没有内核时相关测试自动跳过
go test -v ./...
```

测试内容：

- 为每种协议生成 mihomo 配置，并用真实内核执行 `mihomo -t` 校验
- 端到端跑通「启动内核 → 通过 mixed-port 代理请求 → 得到延迟」

## 🔗 通过 Gist 订阅（适用于私有仓库）

### 方式一：GitHub Gist（推荐）

1. 配置 `GIST_TOKEN`（或 `GITHUB_TOKEN`），可选配置 `GIST_ID`
2. 运行采集程序，日志中会输出订阅地址：

   ```
   ✅ Gist 已创建，ID: abc123def456...
   🔗 订阅地址: https://gist.githubusercontent.com/<user>/<gist_id>/raw/nodes.txt
   ```

3. 把 `GIST_ID` 加到 Secrets，后续订阅地址保持不变

**Clash / mihomo 客户端中使用：**

```yaml
proxy-providers:
  jms:
    type: http
    url: https://gist.githubusercontent.com/<user>/<gist_id>/raw/nodes.txt
    interval: 3600
    path: ./profiles/jms.yaml
```

### 方式二：仓库 Raw 链接（公开仓库）

```
https://raw.githubusercontent.com/<user>/<repo>/main/nodes.txt
```

私有仓库的 Raw 链接需要认证，请优先使用 Gist 方案。

## 🛠 故障排除

| 现象 | 原因与解决 |
|------|-----------|
| `未找到 mihomo 内核` | 确认 `./mihomo/mihomo`（Linux）或 `./mihomo/mihomo.exe`（Windows）存在；Windows 本地缺内核时运行 `scripts/download-mihomo.ps1` |
| Actions 里报 `cannot execute binary file` | 提交的内核不是 Linux 版，重新运行 `./scripts/download-mihomo.sh --force` 后提交 |
| `git push` 直连 github.com 超时 / `Connection was reset` | git 不会自动读取 Windows 系统代理，需要手动指定：`git config http.proxy http://127.0.0.1:5564`（端口改成你自己代理客户端的混合端口），推送大文件时尤其需要 |
| 仓库 Actions 页面为空 | `.github/workflows/*.yml` 没有提交，或需要在 Actions 页面点击 **Enable Actions** |
| 搜索接口返回 403 | 配置 `GH_PAT`（`public_repo` 权限）；代码搜索本身限流为每分钟 10 次 |
| 所有节点都测速失败 | 检查运行环境能否直连外网；CI runner 在海外，本地网络环境可能不同 |
| 可用节点数为 0 | 关键词失效或订阅源全部失效，检查 `keywords.txt` 与运行日志 |
| 推送失败 | 确认工作流有 `permissions: contents: write`，且分支保护未阻止 Actions 推送 |
| Gist 推送失败 | 确认 Token 有 `gist` 权限 |

## ⚠️ 注意事项

1. **Token 安全**：不要把自己的 Token 提交到仓库，一律使用 Secrets
2. **速率限制**：无 Token 时 GitHub 代码搜索限制为每分钟 10 次请求
3. **采集耗时**：关键词之间会间隔 15 秒以避免限流，全量采集可能耗时较久
4. **内核与平台**：仓库内置 Linux amd64 版内核供 Actions 使用；Windows 版 `mihomo.exe` 不入库，仅本地调试时下载
5. **仅测连通性**：测速只验证「通过该节点能否访问测试地址」，不代表节点长期稳定

## 许可证

本项目遵循项目主仓库的许可证。
