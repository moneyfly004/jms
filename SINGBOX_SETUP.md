# Sing-box 真实链接测速配置指南

## 概述

**默认情况下，程序使用 sing-box 内核进行真实链接测速**（通过代理实际访问网站）。如果你想要使用更快的 TCP 连通性测试，可以禁用 sing-box。

**注意**：sing-box 内核会在 GitHub Actions 中自动下载安装，无需手动操作。

## 优势

- ✅ **真实连接测试**：通过代理实际访问网站，验证节点是否真正可用
- ✅ **更准确的测速**：测试真实的网络延迟，而不仅仅是 TCP 连接
- ✅ **支持所有协议**：sing-box 支持以下所有协议：
  - **Shadowsocks (SS)**
  - **VMess**
  - **VLESS**
  - **Trojan**
  - **ShadowsocksR (SSR)** - 转换为 Shadowsocks 配置
  - **Hysteria / Hysteria2**
  - **WireGuard**
  - **TUIC**

## 配置步骤

### 1. 默认配置（推荐）

**sing-box 默认已启用**，无需额外配置。工作流会自动：
- 下载并安装 sing-box
- 使用真实链接测速
- 支持所有协议类型

### 2. 可选配置

如果需要自定义，可以在 GitHub Secrets 中设置：

访问：https://github.com/moneyfly004/jms/settings/secrets/actions

| Secret 名称 | 值 | 说明 |
|------------|-----|------|
| `USE_SINGBOX` | `true`（默认）或 `false` | 是否启用 sing-box 测速 |
| `TEST_URL` | `http://www.google.com/generate_204` | 测试 URL（可选） |
| `TEST_SPEED` | `false` | 是否进行速度测试（可选） |

### 2. 工作流自动安装 sing-box

工作流会自动下载并安装 sing-box，无需手动操作。

**安装的版本**：`1.10.0-alpha.29`（Linux amd64）

### 3. 运行工作流

配置完成后，运行采集工作流：
1. 访问：https://github.com/moneyfly004/jms/actions
2. 选择 "自动采集节点"
3. 点击 "Run workflow"

## 工作原理

1. **创建配置**：为每个节点创建 sing-box 配置文件
2. **启动代理**：启动 sing-box 作为本地代理
3. **真实测试**：通过代理访问测试 URL（如 `http://www.google.com/generate_204`）
4. **测量延迟**：记录真实的网络延迟
5. **验证结果**：只有成功访问的节点才会被保存

## 环境变量说明

| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `USE_SINGBOX` | 是否使用 sing-box 测速 | `true`（启用 sing-box，设置为 `false` 使用 TCP 测试） |
| `SINGBOX_PATH` | sing-box 可执行文件路径 | `/usr/local/bin/sing-box` |
| `TEST_URL` | 测试 URL | `http://www.google.com/generate_204` |
| `TEST_SPEED` | 是否进行速度测试 | `false` |
| `TEST_TIMEOUT` | 测试超时时间（秒） | `10` |

## 测试 URL 推荐

- `http://www.google.com/generate_204` - Google 连接检查（推荐）
- `http://captive.apple.com/` - Apple 连接检查
- `http://www.msftconnecttest.com/connecttest.txt` - Microsoft 连接检查

## 性能说明

- **sing-box 测速**：更准确，但速度较慢（每个节点需要启动代理）
- **TCP 测试**：速度快，但只能验证端口是否开放

**建议**：
- 如果节点数量较少（< 100），使用 sing-box 测速
- 如果节点数量较多（> 100），使用 TCP 测试

## 故障排除

### 问题：sing-box 未找到

**解决方案**：
- 检查工作流日志，确认 sing-box 是否成功安装
- 确认 `SINGBOX_PATH` 环境变量是否正确

### 问题：测试超时

**解决方案**：
- 增加 `TEST_TIMEOUT` 环境变量值
- 更换 `TEST_URL` 为更快的测试地址

### 问题：代理连接失败

**解决方案**：
- 检查节点配置是否正确
- 某些节点可能需要特定的网络环境

## 禁用 sing-box（使用 TCP 测试）

如果不想使用 sing-box（例如节点数量很多，需要更快速度），只需：
- 在 GitHub Secrets 中设置 `USE_SINGBOX` = `false`
- 程序会自动回退到 TCP 连通性测试（速度更快，但只能验证端口是否开放）

