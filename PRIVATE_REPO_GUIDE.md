# 私有仓库订阅方案

## 方案说明

你的需求：
- ✅ **仓库保持私有**：代码完全不可见
- ✅ **获得订阅地址**：可以订阅节点信息

## 当前方案：GitHub Gist

### 工作原理

1. **仓库完全私有**
   - 你的代码仓库 `moneyfly004/jms` 保持私有
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

### 隐私保护

| 内容 | 可见性 | 说明 |
|------|--------|------|
| 代码仓库 | 🔒 私有 | 完全不可见 |
| 采集代码 | 🔒 私有 | 完全不可见 |
| 节点列表 | 🌐 公开 | 仅节点文件，不包含代码 |
| 订阅地址 | 🌐 公开 | 仅用于订阅节点 |

**结论**：你的代码完全隐藏，只有节点列表是公开的（这是订阅必需的）。

## 快速设置

### 步骤 1：配置 Secrets

访问：https://github.com/moneyfly004/jms/settings/secrets/actions

确保已设置：
- `GITHUB_TOKEN` 或 `GIST_TOKEN`（用于创建/更新 Gist）

### 步骤 2：运行工作流

1. 访问：https://github.com/moneyfly004/jms/actions
2. 选择 "自动采集节点"
3. 点击 "Run workflow"

### 步骤 3：获取订阅地址

工作流运行完成后，在日志中查找：

```
✅ Gist 已创建，ID: abc123def456...
🔗 订阅地址: https://gist.githubusercontent.com/moneyfly004/abc123def456.../raw/nodes.txt
```

**复制这个订阅地址**，这就是你的订阅链接。

### 步骤 4：保存 Gist ID（可选）

第一次运行后，会显示 Gist ID。你可以：

1. 在 Secrets 中添加 `GIST_ID` = `abc123def456...`
2. 这样后续更新会使用同一个 Gist，订阅地址不变

## 使用订阅

### 在客户端中添加

**Clash**:
```yaml
proxy-providers:
  jms:
    type: http
    url: https://gist.githubusercontent.com/moneyfly004/{gist_id}/raw/nodes.txt
    interval: 3600
```

**V2Ray / Shadowsocks**:
直接添加订阅地址：
```
https://gist.githubusercontent.com/moneyfly004/{gist_id}/raw/nodes.txt
```

## 自动更新

- 工作流每天自动运行
- 自动更新 Gist 中的节点
- 订阅地址保持不变
- 客户端自动获取最新节点

## 安全说明

### 你的代码安全吗？

✅ **完全安全**：
- 代码仓库是私有的，只有你能访问
- Gist 只包含节点文件，不包含任何代码
- 别人无法知道你的采集逻辑
- 别人无法知道你的 GitHub Token

### Gist 公开是否安全？

✅ **安全**：
- Gist 只包含节点链接（ss://、vmess:// 等）
- 不包含你的个人信息
- 不包含你的代码
- 不包含你的 Token
- 这些节点本身就是公开的（从公开的 GitHub 代码中采集）

## 替代方案（如果需要完全私有）

如果你希望订阅也完全私有，可以考虑：

1. **本地服务器**：运行订阅服务器，只有你能访问
2. **VPN/内网**：将订阅服务器放在内网
3. **认证订阅**：添加认证机制（需要额外开发）

但对于大多数使用场景，当前的 Gist 方案已经足够安全和私密。

