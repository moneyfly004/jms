# 仓库设置指南

## 1. 推送代码到 GitHub

由于认证问题，你需要使用以下方式之一来推送代码：

### 方式一：使用 Personal Access Token（推荐）

```bash
# 使用 token 作为密码推送
git push -u origin main
# 用户名输入：moneyfly004
# 密码输入：你的 GitHub Personal Access Token
```

### 方式二：使用 SSH

```bash
# 1. 生成 SSH 密钥（如果还没有）
ssh-keygen -t ed25519 -C "your_email@example.com"

# 2. 添加 SSH 密钥到 GitHub
# 复制公钥内容
cat ~/.ssh/id_ed25519.pub

# 3. 在 GitHub 添加 SSH 密钥
# Settings → SSH and GPG keys → New SSH key

# 4. 更改远程仓库地址为 SSH
git remote set-url origin git@github.com:moneyfly004/jms.git

# 5. 推送代码
git push -u origin main
```

### 方式三：使用 GitHub CLI

```bash
# 安装 GitHub CLI 后
gh auth login
git push -u origin main
```

## 2. 配置 GitHub Secrets

推送代码后，需要在 GitHub 仓库中配置 Secrets：

### 步骤

1. 访问仓库：https://github.com/moneyfly004/jms
2. 点击 **Settings** → **Secrets and variables** → **Actions**
3. 点击 **New repository secret** 添加以下 Secrets：

#### 必需的 Secret

| Secret 名称 | 说明 | 如何获取 |
|------------|------|----------|
| `GITHUB_TOKEN` | GitHub Personal Access Token | 见下方说明 |

#### 可选的 Secret

| Secret 名称 | 说明 | 默认值 |
|------------|------|--------|
| `GITHUB_REPO` | 仓库名称 | `moneyfly004/jms` |
| `MAX_CONCURRENCY` | 最大并发数 | `10` |
| `TEST_TIMEOUT` | 节点测试超时（秒） | `5` |

### 获取 GitHub Token

1. 访问 https://github.com/settings/tokens
2. 点击 **Generate new token (classic)**
3. 设置 Token 名称：`jms-collector`
4. 选择过期时间（建议选择较长时间）
5. 勾选以下权限：
   - ✅ `public_repo` - 搜索公开代码
   - ✅ `repo` - 推送代码到仓库
6. 点击 **Generate token**
7. **重要**：立即复制 token（只显示一次）
8. 在仓库 Secrets 中添加为 `GITHUB_TOKEN`

## 3. 启用 GitHub Actions

1. 确保代码已推送到 GitHub
2. 访问仓库的 **Actions** 标签页
3. 如果提示需要启用 Actions，点击 **Enable Actions**
4. 工作流会自动在每天 UTC 02:00（北京时间 10:00）运行
5. 也可以手动触发：**Actions** → **自动采集节点** → **Run workflow**

## 4. 验证自动化

1. 等待第一次自动运行完成，或手动触发一次
2. 检查 **Actions** 标签页的运行日志
3. 如果成功，`nodes.txt` 文件会自动更新
4. 查看提交历史，应该能看到自动提交的记录

## 故障排除

### 推送失败：Permission denied

- 检查 GitHub 账户是否正确
- 使用 Personal Access Token 或 SSH 密钥
- 确认有仓库的写入权限

### Actions 运行失败：Token 错误

- 检查 `GITHUB_TOKEN` Secret 是否正确设置
- 确认 Token 有 `repo` 和 `public_repo` 权限
- Token 是否已过期

### Actions 无法推送代码

- 检查工作流的 `permissions` 配置
- 确认 `GITHUB_TOKEN` 有写入权限
- 查看 Actions 运行日志中的错误信息

