# JMS 节点采集器

自动化采集 GitHub 上包含 `jmssub.net` 和 `jjsubmarines.com` 关键词的代码资源，提取订阅链接，解析节点并进行测速，最终保存可用节点。

订阅地址 https://gist.githubusercontent.com/moneyfly004/e2b2b5f89928dcb48a62d6394504a324/raw/nodes.txt

## 🚀 快速开始

### 首次设置

如果你是第一次使用，请查看 [SETUP.md](SETUP.md) 了解如何：
- 推送代码到 GitHub
- 配置 GitHub Secrets
- 启用自动化采集

### GitHub Actions 自动化

本项目已配置 GitHub Actions，可以**每天自动采集节点**并更新到仓库。

**设置步骤：**
1. 在仓库 Settings → Secrets 中添加 `GITHUB_TOKEN`
2. 启用 GitHub Actions
3. 工作流会自动每天运行

详细说明请查看 [SETUP.md](SETUP.md)

## 功能特性

- 🔍 **GitHub 代码搜索**: 自动搜索包含目标关键词的代码
- 🔗 **链接提取**: 从搜索结果中提取订阅链接
- 📦 **节点解析**: 解析 base64 编码的订阅内容，提取 ss 和 vmess 节点
- ⚡ **节点测速**: 对节点进行 TCP 连通性测试
- 💾 **结果保存**: 保存可用节点到本地文件
- 🚀 **GitHub 推送**: 可选地将结果推送到 GitHub 仓库

## 使用方法

### 基本使用（不需要 GitHub Token）

```bash
cd jms采集
go run main.go config.go node_parser.go
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
go run main.go config.go node_parser.go
```

### 推送到 GitHub 仓库

如果你想将结果自动推送到 GitHub 仓库：

```bash
export GITHUB_TOKEN=your_github_token
export GITHUB_REPO=your_username/your_repo
go run main.go config.go node_parser.go
```

程序会将 `nodes.txt` 推送到指定仓库的根目录。

### 高级配置

```bash
# 设置并发数（默认 10）
export MAX_CONCURRENCY=20

# 设置节点测试超时时间（秒，默认 5）
export TEST_TIMEOUT=10

# 完整示例
export GITHUB_TOKEN=your_token
export GITHUB_REPO=username/repo
export MAX_CONCURRENCY=20
export TEST_TIMEOUT=10
go run main.go config.go node_parser.go
```

## 环境变量说明

| 变量名 | 说明 | 必需 | 默认值 |
|--------|------|------|--------|
| `GITHUB_TOKEN` | GitHub Personal Access Token | 否 | - |
| `GITHUB_REPO` | GitHub 仓库（格式: owner/repo） | 否 | - |
| `MAX_CONCURRENCY` | 最大并发数 | 否 | 10 |
| `TEST_TIMEOUT` | 节点测试超时（秒） | 否 | 5 |

## 获取 GitHub Token

1. 访问 https://github.com/settings/tokens
2. 点击 "Generate new token (classic)"
3. 选择权限：
   - `public_repo` (如果需要推送)
   - `repo` (如果需要推送到私有仓库)
4. 生成并复制 token

**注意**: 即使不提供 token，程序也可以运行，但会受到 GitHub API 的速率限制。

## 输出文件

程序会在当前目录生成 `nodes.txt` 文件，每行一个节点链接，格式如下：

```
ss://YWVzLTI1Ni1nY206YzVUYzNGN1c0NFdjQmRFREA5Ni40NS4xODguMzM6MTUxMzA#JMS-1268850@c83s1.portablesubmarines.com:15130
vmess://eyJwcyI6IkpNUy0xMjY4ODUwQGM4M3MzLnBvcnRhYmxlc3VibWFyaW5lcy5jb206MTUxMzAiLCJwb3J0IjoiMTUxMzAiLCJpZCI6ImRkZjg4ZjQxLWMyM2UtNDZhMC04MGZhLTA2MmJiOTBiYzg0OCIsImFpZCI6MCwibmV0IjoidGNwIiwidHlwZSI6Im5vbmUiLCJ0bHMiOiJub25lIiwiYWRkIjoiMTk4LjM1LjQ3LjI3In0
...
```

## 自动化运行

### 使用 GitHub Actions（推荐）

本项目已配置 GitHub Actions 工作流，可以自动每天采集节点并更新到仓库。

#### 设置步骤

1. **配置 GitHub Secrets**

   在仓库设置中添加以下 Secrets（Settings → Secrets and variables → Actions → New repository secret）：

   | Secret 名称 | 说明 | 必需 | 示例值 |
   |------------|------|------|--------|
   | `GITHUB_TOKEN` | GitHub Personal Access Token（用于搜索和推送） | 是 | `ghp_xxxxxxxxxxxx` |
   | `GITHUB_REPO` | 仓库名称（格式: owner/repo） | 否 | `moneyfly004/jms` |
   | `MAX_CONCURRENCY` | 最大并发数 | 否 | `10` |
   | `TEST_TIMEOUT` | 节点测试超时（秒） | 否 | `5` |

   **如何获取 GITHUB_TOKEN：**
   1. 访问 https://github.com/settings/tokens
   2. 点击 "Generate new token (classic)"
   3. 选择权限：
      - `public_repo` (搜索代码)
      - `repo` (推送代码到仓库)
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
0 2 * * * cd /path/to/goweb/jms采集 && /usr/local/go/bin/go run main.go config.go node_parser.go >> /path/to/logs/jms_collector.log 2>&1
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
ExecStart=/usr/local/go/bin/go run main.go config.go node_parser.go

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

## 注意事项

1. **速率限制**: 不使用 token 时，GitHub API 限制为每分钟 10 次请求
2. **网络连接**: 节点测速需要网络连接，某些节点可能无法访问
3. **节点有效性**: 程序只进行基本的 TCP 连通性测试，不保证节点完全可用
4. **隐私**: 请妥善保管你的 GitHub Token，不要提交到代码仓库

## 订阅节点

采集的节点可以通过以下方式订阅：

### 方式一：GitHub Gist（推荐，支持私有仓库）

如果仓库是私有的，使用 GitHub Gist 提供订阅：

1. 在仓库 Secrets 中设置 `GIST_TOKEN`（或使用 `GITHUB_TOKEN`）
2. 可选：设置 `GIST_ID`（如果已有 Gist）
3. 工作流会自动创建/更新 Gist
4. 订阅地址格式：`https://gist.githubusercontent.com/{username}/{gist_id}/raw/nodes.txt`

详细设置请查看 [GIST_SETUP.md](GIST_SETUP.md)

### 方式二：GitHub Raw 链接（公开仓库）

如果仓库是公开的，可以直接使用：
```
https://raw.githubusercontent.com/moneyfly004/jms/main/nodes.txt
```

### 方式三：GitHub Pages（仅公开仓库，私有仓库不可用）

启用 GitHub Pages 后，订阅地址：
```
https://moneyfly004.github.io/jms/nodes.txt
```

**⚠️ 重要**：
- 私有仓库的 GitHub Pages 在免费账户中**不可用**
- 如果看到 Pages 工作流失败，这是正常的
- **请使用 Gist 方案**（方式一），这是私有仓库的最佳选择
- 详细说明见 [PAGES_ISSUE.md](PAGES_ISSUE.md)

详细订阅说明请查看 [SUBSCRIPTION.md](SUBSCRIPTION.md)

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
   - 建议使用 GitHub Pages 或本地订阅服务器
   - 详细说明见 [SUBSCRIPTION.md](SUBSCRIPTION.md)

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
- 使用 GitHub Pages（推荐）
- 或运行本地订阅服务器
- 详细说明见 [SUBSCRIPTION.md](SUBSCRIPTION.md)

## 许可证

本项目遵循项目主仓库的许可证。
