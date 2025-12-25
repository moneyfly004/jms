# 配置 Gist ID 快速指南

## 你的 Gist ID

```
e2b2b5f89928dcb48a62d6394504a324
```

## 设置步骤

### 1. 访问 GitHub Secrets 设置

访问：https://github.com/moneyfly004/jms/settings/secrets/actions

### 2. 添加 GIST_ID Secret

1. 点击 **"New repository secret"**
2. **Name** 填写：`GIST_ID`
3. **Secret** 填写：`e2b2b5f89928dcb48a62d6394504a324`
4. 点击 **"Add secret"**

### 3. 确认 Token 已设置

确保已设置以下 Secret 之一：
- `GIST_TOKEN`（推荐）
- 或 `GITHUB_TOKEN`（程序会自动使用）

**如何获取 Token：**
1. 访问：https://github.com/settings/tokens
2. 点击 "Generate new token (classic)"
3. 选择权限：`gist`（创建和更新 Gist）
4. 生成并复制 token
5. 在 Secrets 中添加为 `GIST_TOKEN` 或 `GITHUB_TOKEN`

### 4. 运行工作流

配置完成后，运行采集工作流：
1. 访问：https://github.com/moneyfly004/jms/actions
2. 选择 "自动采集节点"
3. 点击 "Run workflow"

## 订阅地址

配置完成后，你的订阅地址将是：

```
https://gist.githubusercontent.com/moneyfly004/e2b2b5f89928dcb48a62d6394504a324/raw/nodes.txt
```

或者访问 Gist 页面：
```
https://gist.github.com/moneyfly004/e2b2b5f89928dcb48a62d6394504a324
```

## 验证

1. 运行工作流后，查看日志
2. 应该看到：`✅ 已成功推送到 GitHub Gist`
3. 访问订阅地址，应该能看到节点内容

## 自动更新

配置完成后：
- 工作流每天自动运行
- 自动更新这个 Gist 中的节点
- 订阅地址保持不变
- 只保存测试通过的节点

## 注意事项

- Gist 必须是**公开的**（public），才能作为订阅地址
- 如果 Gist 是私有的，需要改为公开
- 确保 Token 有 `gist` 权限

