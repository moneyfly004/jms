#!/usr/bin/env bash
# 下载 mihomo（原 Clash.Meta）Linux/macOS 内核到 ./mihomo/
#
# GitHub Actions 真正使用的是 Linux amd64 版，仓库里已经内置了
# ./mihomo/mihomo，因此这个脚本只在文件缺失（或需要升级）时才需要执行。
#
# 用法: ./scripts/download-mihomo.sh [版本] [--force]
#   版本默认 v1.19.31，可用环境变量 MIHOMO_VERSION 覆盖
set -euo pipefail

MIHOMO_VERSION="${MIHOMO_VERSION:-v1.19.31}"
FORCE=0

for arg in "$@"; do
  case "${arg}" in
    --force) FORCE=1 ;;
    v*) MIHOMO_VERSION="${arg}" ;;
    *) echo "❌ 未知参数: ${arg}" >&2; exit 1 ;;
  esac
done

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEST_DIR="${PROJECT_ROOT}/mihomo"
DEST_FILE="${DEST_DIR}/mihomo"

if [ -x "${DEST_FILE}" ] && [ "${FORCE}" -ne 1 ]; then
  echo "✅ 已存在 ${DEST_FILE}，跳过下载（需要覆盖请加 --force）"
  "${DEST_FILE}" -v
  exit 0
fi

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "${os}" in
  linux)  target_os="linux" ;;
  darwin) target_os="darwin" ;;
  *) echo "❌ 不支持的系统: ${os}（Windows 请使用 scripts/download-mihomo.ps1）" >&2; exit 1 ;;
esac

arch="$(uname -m)"
case "${arch}" in
  x86_64|amd64) target_arch="amd64" ;;
  aarch64|arm64) target_arch="arm64" ;;
  *) echo "❌ 不支持的架构: ${arch}" >&2; exit 1 ;;
esac

asset="mihomo-${target_os}-${target_arch}-${MIHOMO_VERSION}.gz"
url="https://github.com/MetaCubeX/mihomo/releases/download/${MIHOMO_VERSION}/${asset}"

mkdir -p "${DEST_DIR}"

echo "⬇️  下载 mihomo ${MIHOMO_VERSION} (${target_os}/${target_arch})"
echo "    ${url}"

tmp="$(mktemp -d)"
trap 'rm -rf "${tmp}"' EXIT

if command -v curl >/dev/null 2>&1; then
  curl -fL --retry 3 --retry-delay 2 -o "${tmp}/${asset}" "${url}"
elif command -v wget >/dev/null 2>&1; then
  wget -O "${tmp}/${asset}" "${url}"
else
  echo "❌ 需要 curl 或 wget" >&2
  exit 1
fi

gunzip -c "${tmp}/${asset}" > "${DEST_FILE}"
chmod +x "${DEST_FILE}"

echo "✅ mihomo 已安装到 ${DEST_FILE}"
"${DEST_FILE}" -v
