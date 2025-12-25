# JMS 节点采集器

自动化采集 GitHub 上包含 `jmssub.net` 和 `jjsubmarines.com` 关键词的代码资源，提取订阅链接，解析节点并进行测速，最终保存可用节点。

## 📡 订阅地址

### JMS 节点订阅（nodes.txt）
```
https://gist.githubusercontent.com/moneyfly004/e2b2b5f89928dcb48a62d6394504a324/raw/nodes.txt
```

### 订阅链接节点订阅（sub.txt）
```
https://gist.githubusercontent.com/moneyfly004/fa0e0bfff22b50d9ca9d92b82ebe2f7c/raw/sub.txt
```

将这些地址添加到你的代理客户端（Clash、V2Ray、Shadowsocks 等）即可自动获取最新节点。

## 🚀 快速开始

### 最简单的使用方式（不需要 GitHub Token）

```bash
cd jms采集
go run main.go config.go node_parser.go gist.go
```

程序会自动：
1. 搜索 GitHub 上包含 `jmssub.net` 和 `jjsubmarines.com` 的代码
2. 提取订阅链接
3. 解析节点并测试连通性
4. 保存可用节点到 `nodes.txt`

### 使用 GitHub Token（推荐，更快）

```bash
export GITHUB_TOKEN=your_token_here
go run main.go config.go node_parser.go gist.go
```

### 推送到 GitHub 仓库

```bash
export GITHUB_TOKEN=your_token_here
export GITHUB_REPO=your_username/your_repo
go run main.go config.go node_parser.go gist.go
```

### 编译为可执行文件

```bash
go build -o jms-collector main.go config.go node_parser.go gist.go
./jms-collector
```

## 功能特性

- 🔍 **GitHub 代码搜索**: 自动搜索包含目标关键词的代码
- 🔗 **链接提取**: 从搜索结果中提取订阅链接
- 📦 **节点解析**: 解析 base64 编码的订阅内容，提取 ss、vmess、vless、trojan、ssr 节点
- ⚡ **节点测速**: 对节点进行 TCP 连通性测试
- 💾 **结果保存**: 默认只保存测试通过的节点
- 🚀 **GitHub 推送**: 可选地将结果推送到 GitHub 仓库或 Gist

## 使用方法

### 基本使用（不需要 GitHub Token）

```bash
cd jms采集
go run main.go config.go node_parser.go gist.go
```

程序会：
1. 使用 GitHub 公开 API 搜索代码（有速率限制，每分钟 10 次）
2. 提取订阅链接
3. 解析并测试节点
4. 保存结果到 `nodes.txt`

### 使用 GitHub Token（推荐）

使用 GitHub Token 可以获得更高的 API 速率限制（每分钟 5000 次）。

```bash
export GITHUB_TOKEN=your_github_token
go run main.go config.go node_parser.go gist.go
```

### 推送到 GitHub 仓库

如果你想将结果自动推送到 GitHub 仓库：

```bash
export GITHUB_TOKEN=your_github_token
export GITHUB_REPO=your_username/your_repo
go run main.go config.go node_parser.go gist.go
```

程序会将 `nodes.txt` 推送到指定仓库的根目录。

### 高级配置

```bash
# 设置并发数（默认 10）
export MAX_CONCURRENCY=20

# 设置节点测试超时时间（秒，默认 10）
export TEST_TIMEOUT=10

# 是否保存所有节点（包括测试失败的），默认 false（只保存测试通过的）
export SAVE_ALL_NODES=false

# 完整示例
export GITHUB_TOKEN=your_token
export GITHUB_REPO=username/repo
export MAX_CONCURRENCY=20
export TEST_TIMEOUT=10
export SAVE_ALL_NODES=false
go run main.go config.go node_parser.go gist.go
```

## 环境变量说明

| 变量名 | 说明 | 必需 | 默认值 |
|--------|------|------|--------|
| `GITHUB_TOKEN` | GitHub Personal Access Token | 否 | - |
| `GITHUB_REPO` | GitHub 仓库（格式: owner/repo） | 否 | - |
| `GIST_ID` | GitHub Gist ID（用于更新现有 Gist） | 否 | - |
| `GIST_TOKEN` | GitHub Token（用于创建/更新 Gist） | 否 | 使用 `GITHUB_TOKEN` |
| `MAX_CONCURRENCY` | 最大并发数 | 否 | 10 |
| `TEST_TIMEOUT` | 节点测试超时（秒） | 否 | 10 |
| `SAVE_ALL_NODES` | 是否保存所有节点（包括测试失败的） | 否 | false |

## 获取 GitHub Token

1. 访问 https://github.com/settings/tokens
2. 点击 "Generate new token (classic)"
3. 选择权限：
   - `public_repo` (如果需要搜索和推送)
   - `repo` (如果需要推送到私有仓库)
   - `gist` (如果需要创建/更新 Gist)
4. 生成并复制 token

**注意**: 即使不提供 token，程序也可以运行，但会受到 GitHub API 的速率限制。

## 输出文件

程序会在当前目录生成 `nodes.txt` 文件，每行一个节点链接，格式如下：

```
ss://YWVzLTI1Ni1nY206YzVUYzNGN1c0NFdjQmRFREA5Ni40NS4xODguMzM6MTUxMzA#JMS-1268850@c83s1.portablesubmarines.com:15130
vmess://eyJwcyI6IkpNUy0xMjY4ODUwQGM4M3MzLnBvcnRhYmxlc3VibWFyaW5lcy5jb206MTUxMzAiLCJwb3J0IjoiMTUxMzAiLCJpZCI6ImRkZjg4ZjQxLWMyM2UtNDZhMC04MGZhLTA2MmJiOTBiYzg0OCIsImFpZCI6MCwibmV0IjoidGNwIiwidHlwZSI6Im5vbmUiLCJ0bHMiOiJub25lIiwiYWRkIjoiMTk4LjM1LjQ3LjI3In0
...
```

## 仓库设置指南

### 1. 推送代码到 GitHub

由于认证问题，你需要使用以下方式之一来推送代码：

#### 方式一：使用 Personal Access Token（推荐）

```bash
# 使用 token 作为密码推送
git push -u origin main
# 用户名输入：你的 GitHub 用户名
# 密码输入：你的 GitHub Personal Access Token
```

#### 方式二：使用 SSH

```bash
# 1. 生成 SSH 密钥（如果还没有）
ssh-keygen -t ed25519 -C "your_email@example.com"

# 2. 添加 SSH 密钥到 GitHub
# 复制公钥内容
cat ~/.ssh/id_ed25519.pub

# 3. 在 GitHub 添加 SSH 密钥
# Settings → SSH and GPG keys → New SSH key

# 4. 更改远程仓库地址为 SSH
git remote set-url origin git@github.com:your_username/your_repo.git

# 5. 推送代码
git push -u origin main
```

#### 方式三：使用 GitHub CLI

```bash
# 安装 GitHub CLI 后
gh auth login
git push -u origin main
```

### 2. 配置 GitHub Secrets

推送代码后，需要在 GitHub 仓库中配置 Secrets：

#### 步骤

1. 访问仓库：https://github.com/your_username/your_repo
2. 点击 **Settings** → **Secrets and variables** → **Actions**
3. 点击 **New repository secret** 添加以下 Secrets：

#### 必需的 Secret

| Secret 名称 | 说明 | 如何获取 |
|------------|------|----------|
| `GITHUB_TOKEN` | GitHub Personal Access Token | 见下方说明 |

#### 可选的 Secret

| Secret 名称 | 说明 | 默认值 |
|------------|------|--------|
| `GITHUB_REPO` | 仓库名称 | `your_username/your_repo` |
| `GIST_ID` | Gist ID（如果设置则更新现有 Gist） | - |
| `GIST_TOKEN` | GitHub Token（用于创建/更新 Gist） | 使用 `GITHUB_TOKEN` |
| `MAX_CONCURRENCY` | 最大并发数 | `10` |
| `TEST_TIMEOUT` | 节点测试超时（秒） | `10` |
| `SAVE_ALL_NODES` | 是否保存所有节点（包括测试失败的） | `false` |

#### 获取 GitHub Token

1. 访问 https://github.com/settings/tokens
2. 点击 **Generate new token (classic)**
3. 设置 Token 名称：`jms-collector`
4. 选择过期时间（建议选择较长时间）
5. 勾选以下权限：
   - ✅ `public_repo` - 搜索公开代码
   - ✅ `repo` - 推送代码到仓库
   - ✅ `gist` - 创建/更新 Gist（如果使用 Gist 订阅）
6. 点击 **Generate token**
7. **重要**：立即复制 token（只显示一次）
8. 在仓库 Secrets 中添加为 `GITHUB_TOKEN`

### 3. 启用 GitHub Actions

1. 确保代码已推送到 GitHub
2. 访问仓库的 **Actions** 标签页
3. 如果提示需要启用 Actions，点击 **Enable Actions**
4. 工作流会自动在每天 UTC 02:00（北京时间 10:00）运行
5. 也可以手动触发：**Actions** → **自动采集节点** → **Run workflow**

### 4. 验证自动化

1. 等待第一次自动运行完成，或手动触发一次
2. 检查 **Actions** 标签页的运行日志
3. 如果成功，`nodes.txt` 文件会自动更新
4. 查看提交历史，应该能看到自动提交的记录

## 自动化运行

### 使用 GitHub Actions（推荐）

本项目已配置 GitHub Actions 工作流，可以自动每天采集节点并更新到仓库。

#### 设置步骤

1. **配置 GitHub Secrets**

   在仓库设置中添加以下 Secrets（Settings → Secrets and variables → Actions → New repository secret）：

   | Secret 名称 | 说明 | 必需 | 示例值 |
   |------------|------|------|--------|
   | `GITHUB_TOKEN` | GitHub Personal Access Token（用于搜索和推送） | 是 | `ghp_xxxxxxxxxxxx` |
   | `GITHUB_REPO` | 仓库名称（格式: owner/repo） | 否 | `your_username/your_repo` |
   | `GIST_ID` | Gist ID（用于更新现有 Gist） | 否 | `e2b2b5f89928dcb48a62d6394504a324` |
   | `GIST_TOKEN` | GitHub Token（用于创建/更新 Gist） | 否 | 使用 `GITHUB_TOKEN` |
   | `MAX_CONCURRENCY` | 最大并发数 | 否 | `10` |
   | `TEST_TIMEOUT` | 节点测试超时（秒） | 否 | `10` |
   | `SAVE_ALL_NODES` | 是否保存所有节点（包括测试失败的） | 否 | `false` |

   **如何获取 GITHUB_TOKEN：**
   1. 访问 https://github.com/settings/tokens
   2. 点击 "Generate new token (classic)"
   3. 选择权限：
      - `public_repo` (搜索代码)
      - `repo` (推送代码到仓库)
      - `gist` (创建/更新 Gist)
   4. 生成并复制 token
   5. 在仓库 Settings → Secrets 中添加为 `GITHUB_TOKEN`

2. **启用 GitHub Actions**

   - 工作流文件已创建在 `.github/workflows/collect-nodes.yml`
   - 默认每天 UTC 时间 02:00（北京时间 10:00）自动运行
   - 也可以手动触发：Actions → 选择 "自动采集节点" → Run workflow

3. **查看运行结果**

   - 在 Actions 标签页查看运行日志
   - 采集的节点会自动保存到 `nodes.txt` 文件
   - 如果有更新，会自动提交并推送到仓库

#### 工作流说明

- **定时触发**: 每天 UTC 02:00 自动运行
- **手动触发**: 可以在 Actions 页面手动运行
- **自动提交**: 如果节点有更新，会自动提交并推送

### 使用 cron（Linux/macOS）

```bash
# 编辑 crontab
crontab -e

# 添加定时任务（每天凌晨 2 点运行）
0 2 * * * cd /path/to/goweb/jms采集 && /usr/local/go/bin/go run main.go config.go node_parser.go gist.go >> /path/to/logs/jms_collector.log 2>&1
```

### 使用 systemd（Linux）

创建服务文件 `/etc/systemd/system/jms-collector.service`:

```ini
[Unit]
Description=JMS Node Collector
After=network.target

[Service]
Type=oneshot
User=your_user
WorkingDirectory=/path/to/goweb/jms采集
Environment="GITHUB_TOKEN=your_token"
Environment="GITHUB_REPO=username/repo"
ExecStart=/usr/local/go/bin/go run main.go config.go node_parser.go gist.go

[Install]
WantedBy=multi-user.target
```

创建定时器 `/etc/systemd/system/jms-collector.timer`:

```ini
[Unit]
Description=Run JMS Collector Daily
Requires=jms-collector.service

[Timer]
OnCalendar=daily
OnCalendar=02:00

[Install]
WantedBy=timers.target
```

启用定时器：

```bash
sudo systemctl enable jms-collector.timer
sudo systemctl start jms-collector.timer
```

## 订阅节点

采集的节点可以通过以下方式订阅：

### 方式一：GitHub Gist（推荐，支持私有仓库）

**为什么使用 Gist？**

- ✅ **完全免费**：GitHub Gist 对所有人免费
- ✅ **公开访问**：Gist 是公开的，无需认证即可访问
- ✅ **自动更新**：可以通过 API 自动更新内容
- ✅ **简单易用**：订阅地址格式简单
- ✅ **支持私有仓库**：即使仓库是私有的，Gist 也可以公开访问

**设置步骤：**

1. **配置 GitHub Secrets**

   在仓库设置中添加以下 Secrets：

   | Secret 名称 | 说明 | 必需 |
   |------------|------|------|
   | `GIST_TOKEN` | GitHub Token（用于创建/更新 Gist） | 是（或使用 `GITHUB_TOKEN`） |
   | `GIST_ID` | Gist ID（可选，如果设置则更新现有 Gist） | 否 |

   **注意**：如果没有设置 `GIST_ID`，程序会自动创建新的 Gist，并在日志中显示 Gist ID。

   **获取 GIST_TOKEN：**
   - `GIST_TOKEN` 可以使用你的 `GITHUB_TOKEN`（如果已经有的话）
   - 或者单独创建一个只有 `gist` 权限的 Token

2. **运行采集程序**

   配置完成后，GitHub Actions 会自动运行，或者你可以手动触发：
   - 访问：https://github.com/your_username/your_repo/actions
   - 选择 "自动采集节点" 工作流
   - 点击 "Run workflow"

3. **获取订阅地址**

   工作流运行完成后，查看日志，你会看到类似这样的输出：

   ```
   ✅ Gist 已创建，ID: abc123def456...
   🔗 订阅地址: https://gist.githubusercontent.com/your_username/abc123def456.../raw/nodes.txt
   🌐 Gist 页面: https://gist.github.com/your_username/abc123def456...
   ```

   **订阅地址格式**：
   ```
   https://gist.githubusercontent.com/{username}/{gist_id}/raw/nodes.txt
   ```

4. **保存 Gist ID（可选）**

   第一次运行后，会显示 Gist ID。你可以：
   - 在 Secrets 中添加 `GIST_ID` = `abc123def456...`
   - 这样后续更新会使用同一个 Gist，订阅地址不变

**自动更新：**

配置完成后：
- 采集工作流每天自动运行
- 自动更新 Gist 中的节点内容
- 订阅地址保持不变，始终指向最新的节点

**在客户端中使用：**

**Clash 配置**：
```yaml
proxy-providers:
  jms:
    type: http
    url: https://gist.githubusercontent.com/your_username/{gist_id}/raw/nodes.txt
    interval: 3600
    path: ./profiles/jms.yaml
```

**V2Ray / Shadowsocks**：
直接在客户端中添加订阅地址：
```
https://gist.githubusercontent.com/your_username/{gist_id}/raw/nodes.txt
```

### 方式二：GitHub Raw 链接（公开仓库）

如果仓库是公开的，可以直接使用：
```
https://raw.githubusercontent.com/your_username/your_repo/main/nodes.txt
```


## 私有仓库支持

### GitHub Actions 在私有仓库中运行

**完全支持！** GitHub Actions 可以在私有仓库中正常运行：

1. **默认配置即可工作**
   - GitHub Actions 自动提供的 `GITHUB_TOKEN` 在私有仓库中有完整权限
   - 无需额外配置即可运行

2. **如果使用自定义 Token**
   - 确保 Token 有 `repo` 权限（完整仓库访问）
   - 搜索公开代码需要 `public_repo` 权限

3. **订阅访问**
   - 私有仓库的 Raw 链接需要认证
   - **推荐使用 GitHub Gist**（方式一），这是私有仓库的最佳选择
   - 详细说明见上方"订阅节点"部分

### 私有仓库订阅方案

**当前方案：GitHub Gist**

**工作原理：**

1. **仓库完全私有**
   - 你的代码仓库保持私有
   - 任何人都无法看到你的代码
   - 只有你能访问仓库

2. **节点通过 Gist 公开**
   - GitHub Actions 自动将节点推送到独立的 Gist
   - Gist 是公开的，但**只包含节点文件**，不包含代码
   - 别人只能看到节点列表，看不到你的采集代码

3. **订阅地址**
   - 格式：`https://gist.githubusercontent.com/{username}/{gist_id}/raw/nodes.txt`
   - 只有这个地址是公开的
   - 代码仓库完全隐藏

**隐私保护：**

| 内容 | 可见性 | 说明 |
|------|--------|------|
| 代码仓库 | 🔒 私有 | 完全不可见 |
| 采集代码 | 🔒 私有 | 完全不可见 |
| 节点列表 | 🌐 公开 | 仅节点文件，不包含代码 |
| 订阅地址 | 🌐 公开 | 仅用于订阅节点 |

**结论**：你的代码完全隐藏，只有节点列表是公开的（这是订阅必需的）。

## 故障排除

### 问题: 搜索失败，返回 403

**解决方案**: 提供 GitHub Token 或等待速率限制重置

### 问题: 节点解析失败

**解决方案**: 检查订阅链接是否有效，内容是否为 base64 编码

### 问题: 推送失败

**解决方案**: 
- 确认 GitHub Token 有 `repo` 权限
- 确认仓库名称格式正确（owner/repo）
- 确认仓库存在且有写入权限

### 问题: 私有仓库无法访问订阅

**解决方案**:
- **使用 GitHub Gist**（推荐，见上方"订阅节点"部分）
- 或运行本地订阅服务器

### 问题: Gist 创建/更新失败

**解决方案**：
- 检查 `GIST_TOKEN` 或 `GITHUB_TOKEN` 是否正确设置
- 确认 Token 有 `gist` 权限
- 查看 Actions 运行日志中的错误信息

### 问题: Gist 节点数量与本地文件不一致

**解决方案**：
- 检查工作流日志，查看 Gist 推送是否成功
- 确认 Gist ID 是否正确
- 查看日志中的节点数量统计


### 问题: Actions 运行失败：Token 错误

**解决方案**：
- 检查 `GITHUB_TOKEN` Secret 是否正确设置
- 确认 Token 有 `repo` 和 `public_repo` 权限
- Token 是否已过期

### 问题: Actions 无法推送代码

**解决方案**：
- 检查工作流的 `permissions` 配置
- 确认 `GITHUB_TOKEN` 有写入权限
- 查看 Actions 运行日志中的错误信息

## 注意事项

1. **速率限制**: 不使用 token 时，GitHub API 限制为每分钟 10 次请求
2. **网络连接**: 节点测速需要网络连接，某些节点可能无法访问
3. **节点有效性**: 程序只进行基本的 TCP 连通性测试，不保证节点完全可用
4. **隐私**: 请妥善保管你的 GitHub Token，不要提交到代码仓库
5. **默认行为**: 程序默认只保存测试通过的节点，测试失败的节点不会被保存
6. **Gist 公开**: Gist 必须是公开的（public），才能作为订阅地址

## 许可证

本项目遵循项目主仓库的许可证。
