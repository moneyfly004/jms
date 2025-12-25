# GitHub Gist 订阅设置指南（私有仓库解决方案）

## 为什么使用 Gist？

- ✅ **仓库完全私有**：你的代码仓库保持私有，完全不可见
- ✅ **仅节点公开**：Gist 只包含节点文件，不包含任何代码
- ✅ **隐私保护**：别人只能看到节点，看不到你的采集代码和逻辑
- ✅ **自动更新**：GitHub Actions 自动更新，订阅地址保持不变

## 为什么使用 Gist？

- ✅ **完全免费**：GitHub Gist 对所有人免费
- ✅ **公开访问**：Gist 是公开的，无需认证即可访问
- ✅ **自动更新**：可以通过 API 自动更新内容
- ✅ **简单易用**：订阅地址格式简单

## 设置步骤

### 步骤 1：创建 GitHub Gist（可选）

如果你想让程序自动创建 Gist，可以跳过此步骤。程序会在第一次运行时自动创建。

如果你想手动创建：

1. 访问：https://gist.github.com
2. 点击 "Create a new gist"
3. 文件名填写：`nodes.txt`
4. 内容可以暂时留空
5. 选择 "Create public gist"
6. 创建后，复制 Gist ID（URL 中的长字符串）

### 步骤 2：配置 GitHub Secrets

在仓库设置中添加以下 Secrets：

1. 访问：https://github.com/moneyfly004/jms/settings/secrets/actions
2. 点击 "New repository secret"

#### 必需的 Secret

| Secret 名称 | 说明 | 如何获取 |
|------------|------|----------|
| `GIST_TOKEN` | GitHub Token（用于创建/更新 Gist） | 见下方说明 |
| `GIST_ID` | Gist ID（可选，如果设置则更新现有 Gist） | Gist URL 中的 ID |

**注意**：如果没有设置 `GIST_ID`，程序会自动创建新的 Gist，并在日志中显示 Gist ID。

#### 获取 GIST_TOKEN

`GIST_TOKEN` 可以使用你的 `GITHUB_TOKEN`（如果已经有的话），或者：

1. 访问：https://github.com/settings/tokens
2. 点击 "Generate new token (classic)"
3. 选择权限：`gist`（创建和更新 Gist）
4. 生成并复制 token
5. 在 Secrets 中添加为 `GIST_TOKEN`

**或者**：如果你已经设置了 `GITHUB_TOKEN`，可以不设置 `GIST_TOKEN`，程序会自动使用 `GITHUB_TOKEN`。

### 步骤 3：运行采集程序

配置完成后，GitHub Actions 会自动运行，或者你可以手动触发：

1. 访问：https://github.com/moneyfly004/jms/actions
2. 选择 "自动采集节点" 工作流
3. 点击 "Run workflow"

### 步骤 4：获取订阅地址

工作流运行完成后，查看日志，你会看到类似这样的输出：

```
✅ Gist 已创建，ID: abc123def456...
🔗 订阅地址: https://gist.githubusercontent.com/moneyfly004/abc123def456.../raw/nodes.txt
🌐 Gist 页面: https://gist.github.com/moneyfly004/abc123def456...
```

**订阅地址格式**：
```
https://gist.githubusercontent.com/{username}/{gist_id}/raw/nodes.txt
```

## 自动更新

配置完成后：
- 采集工作流每天自动运行
- 自动更新 Gist 中的节点内容
- 订阅地址保持不变，始终指向最新的节点

## 在客户端中使用

### Clash 配置

```yaml
proxy-providers:
  jms:
    type: http
    url: https://gist.githubusercontent.com/moneyfly004/{gist_id}/raw/nodes.txt
    interval: 3600
    path: ./profiles/jms.yaml
```

### V2Ray / Shadowsocks

直接在客户端中添加订阅地址：
```
https://gist.githubusercontent.com/moneyfly004/{gist_id}/raw/nodes.txt
```

## 故障排除

### 问题：Gist 创建失败

**解决方案**：
- 检查 `GIST_TOKEN` 是否正确设置
- 确认 Token 有 `gist` 权限
- 查看 Actions 运行日志中的错误信息

### 问题：无法访问订阅地址

**解决方案**：
- 确认 Gist 是公开的（public）
- 检查 Gist ID 是否正确
- 等待几分钟，Gist 可能需要时间同步

### 问题：订阅内容为空

**解决方案**：
- 检查采集工作流是否成功运行
- 查看 `nodes.txt` 文件是否有内容
- 确认 Gist 更新是否成功

## 优势对比

| 方案 | 公开仓库 | 私有仓库 | 免费账户 |
|------|---------|---------|---------|
| GitHub Pages | ✅ | ❌ | ❌ |
| GitHub Gist | ✅ | ✅ | ✅ |
| GitHub Raw | ✅ | ❌ | ✅ |

**结论**：对于私有仓库，GitHub Gist 是最佳选择！

