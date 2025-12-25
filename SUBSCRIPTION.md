# 节点订阅指南

## 订阅方式

### 方式一：使用 GitHub Raw 链接（推荐，适用于公开仓库）

如果你的仓库是公开的，可以直接使用 GitHub Raw 链接作为订阅地址：

```
https://raw.githubusercontent.com/moneyfly004/jms/main/nodes.txt
```

**使用方法：**
1. 在代理客户端（如 Clash、V2Ray、Shadowsocks）中添加订阅
2. 订阅地址填写上面的链接
3. 客户端会自动解析 base64 编码的节点

**注意：** 如果仓库是私有的，需要使用方式二或方式三。

### 方式二：使用 GitHub Pages（适用于公开仓库）

1. 在仓库设置中启用 GitHub Pages
2. 将 `nodes.txt` 文件复制到 `docs/` 目录
3. 订阅地址：
   ```
   https://moneyfly004.github.io/jms/nodes.txt
   ```

### 方式三：使用本地订阅服务器（适用于私有仓库）

如果仓库是私有的，可以运行本地订阅服务器：

```bash
# 设置订阅服务器端口
export SUBSCRIBE_PORT=8080

# 运行采集程序（会自动启动订阅服务器）
go run main.go config.go node_parser.go subscribe.go
```

订阅地址：
```
http://localhost:8080/subscribe
```

**部署到服务器：**
```bash
# 编译程序
go build -o jms-collector main.go config.go node_parser.go subscribe.go

# 运行（后台运行）
nohup ./jms-collector > collector.log 2>&1 &

# 或者使用 systemd 服务
```

### 方式四：使用 GitHub Actions + GitHub Pages（推荐）

创建一个 GitHub Actions 工作流，自动将节点文件发布到 GitHub Pages：

1. 创建 `.github/workflows/pages.yml`（见下方）
2. 在仓库设置中启用 GitHub Pages
3. 订阅地址：`https://moneyfly004.github.io/jms/nodes.txt`

## 私有仓库说明

### GitHub Actions 在私有仓库中的运行

**可以运行！** GitHub Actions 在私有仓库中完全支持，但需要注意：

1. **默认 GITHUB_TOKEN 权限**
   - GitHub Actions 会自动提供 `GITHUB_TOKEN`
   - 在私有仓库中，默认有 `repo` 权限
   - 可以正常读取和写入仓库内容

2. **如果使用自定义 Token**
   - 需要在 Secrets 中设置 `GITHUB_TOKEN`
   - Token 需要有 `repo` 权限（完整仓库访问权限）
   - 如果只搜索公开代码，`public_repo` 权限即可

3. **订阅访问问题**
   - 私有仓库的 Raw 链接需要认证才能访问
   - 建议使用 GitHub Pages 或本地订阅服务器
   - 或者将仓库设置为公开（如果节点信息可以公开）

### 推荐方案

**方案 A：公开仓库 + GitHub Raw**
- 最简单，无需额外配置
- 订阅地址：`https://raw.githubusercontent.com/moneyfly004/jms/main/nodes.txt`

**方案 B：私有仓库 + GitHub Pages**
- 需要启用 GitHub Pages
- 订阅地址：`https://moneyfly004.github.io/jms/nodes.txt`

**方案 C：私有仓库 + 本地服务器**
- 完全私有，需要自己维护服务器
- 订阅地址：`http://your-server:8080/subscribe`

## 客户端配置示例

### Clash

```yaml
proxy-providers:
  jms:
    type: http
    url: https://raw.githubusercontent.com/moneyfly004/jms/main/nodes.txt
    interval: 3600
    path: ./profiles/jms.yaml
```

### V2Ray

在 V2Ray 客户端中添加订阅：
- 订阅地址：`https://raw.githubusercontent.com/moneyfly004/jms/main/nodes.txt`
- 更新间隔：1小时

### Shadowsocks

使用订阅转换服务：
1. 访问：https://sub-web.netlify.app/
2. 输入订阅地址
3. 选择客户端类型
4. 生成配置

