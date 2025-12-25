# GitHub Pages 设置指南

## 启用 GitHub Pages

### 步骤 1：在仓库设置中启用 Pages

1. 访问仓库：https://github.com/moneyfly004/jms
2. 点击 **Settings**（设置）
3. 在左侧菜单中找到 **Pages**
4. 在 **Source** 部分：
   - 选择 **GitHub Actions** 作为源
   - 点击 **Save**

### 步骤 2：等待工作流运行

1. 访问 **Actions** 标签页
2. 找到 "发布到 GitHub Pages" 工作流
3. 等待工作流运行完成（通常需要 1-2 分钟）

### 步骤 3：获取订阅地址

工作流运行成功后，你的订阅地址将是：

```
https://moneyfly004.github.io/jms/nodes.txt
```

或者访问主页：
```
https://moneyfly004.github.io/jms/
```

## 自动更新

配置完成后：
- 每次 `nodes.txt` 文件更新时，GitHub Pages 会自动重新发布
- 采集工作流更新节点后，Pages 工作流会自动触发
- 无需手动操作

## 验证

1. 等待 Pages 工作流运行完成
2. 访问：https://moneyfly004.github.io/jms/nodes.txt
3. 应该能看到节点内容（base64 编码）

## 在客户端中使用

### Clash

```yaml
proxy-providers:
  jms:
    type: http
    url: https://moneyfly004.github.io/jms/nodes.txt
    interval: 3600
    path: ./profiles/jms.yaml
```

### V2Ray / Shadowsocks

直接在客户端中添加订阅地址：
```
https://moneyfly004.github.io/jms/nodes.txt
```

## 故障排除

### Pages 工作流没有运行

- 检查仓库 Settings → Actions → General
- 确保 "Allow all actions and reusable workflows" 已启用
- 检查工作流文件 `.github/workflows/pages.yml` 是否存在

### 无法访问订阅地址

- 等待几分钟，Pages 部署需要时间
- 检查 Actions 标签页中的 Pages 工作流是否成功
- 确认仓库不是完全私有的（Pages 需要仓库至少是公开的，或者使用 GitHub Pro）

### 订阅内容为空

- 检查 `nodes.txt` 文件是否有内容
- 查看采集工作流的运行日志
- 确认节点采集是否成功

