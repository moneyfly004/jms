# 快速开始

## 最简单的使用方式（不需要 GitHub Token）

```bash
cd jms采集
go run main.go config.go node_parser.go
```

程序会自动：
1. 搜索 GitHub 上包含 `jmssub.net` 和 `jjsubmarines.com` 的代码
2. 提取订阅链接
3. 解析节点并测试连通性
4. 保存可用节点到 `nodes.txt`

## 使用 GitHub Token（推荐，更快）

```bash
export GITHUB_TOKEN=your_token_here
go run main.go config.go node_parser.go
```

## 推送到 GitHub 仓库

```bash
export GITHUB_TOKEN=your_token_here
export GITHUB_REPO=your_username/your_repo
go run main.go config.go node_parser.go
```

## 编译为可执行文件

```bash
go build -o jms-collector main.go config.go node_parser.go
./jms-collector
```

## 获取 GitHub Token

1. 访问 https://github.com/settings/tokens
2. 点击 "Generate new token (classic)"
3. 选择 `public_repo` 权限（如果只需要搜索）或 `repo` 权限（如果需要推送）
4. 复制生成的 token

**注意**: 即使不提供 token 也可以运行，但会受到速率限制（每分钟 10 次请求）。

