# 下载 mihomo（原 Clash.Meta）Windows 内核到 .\mihomo\mihomo.exe
#
# 注意：GitHub Actions 运行在 Linux 上，使用的是仓库内置的 .\mihomo\mihomo（Linux 版）；
# 这个脚本只是为了让你能在 Windows 本地跑 go run . / go test，产物不入库。
#
# 用法: pwsh -File .\scripts\download-mihomo.ps1 [-Version v1.19.31] [-Force]
param(
    [string]$Version = $env:MIHOMO_VERSION,
    [switch]$Force
)
if ([string]::IsNullOrWhiteSpace($Version)) { $Version = 'v1.19.31' }

$ErrorActionPreference = 'Stop'
$ProjectRoot = Split-Path -Parent $PSScriptRoot
$DestDir = Join-Path $ProjectRoot 'mihomo'
New-Item -ItemType Directory -Force -Path $DestDir | Out-Null
$DestFile = Join-Path $DestDir 'mihomo.exe'

if ((Test-Path $DestFile) -and -not $Force) {
    Write-Host "✅ 已存在 $DestFile，跳过下载（需要覆盖请加 -Force）"
    & $DestFile -v
    exit 0
}

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    'x86'   { '386' }
    default { throw "不支持的架构: $env:PROCESSOR_ARCHITECTURE" }
}

$asset = "mihomo-windows-$arch-$Version.zip"
$url = "https://github.com/MetaCubeX/mihomo/releases/download/$Version/$asset"

Write-Host "⬇️  下载 mihomo $Version (windows/$arch)"
Write-Host "    $url"

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("mihomo-" + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force -Path $tmp | Out-Null

try {
    $zip = Join-Path $tmp $asset
    Invoke-WebRequest -Uri $url -OutFile $zip -UseBasicParsing

    $extract = Join-Path $tmp 'extract'
    Expand-Archive -Path $zip -DestinationPath $extract -Force

    $exe = Get-ChildItem -Path $extract -Recurse -Filter 'mihomo.exe' | Select-Object -First 1
    if (-not $exe) {
        $exe = Get-ChildItem -Path $extract -Recurse -File |
               Where-Object { $_.Name -notmatch '\.(txt|md|dat|metadb)$' } |
               Select-Object -First 1
    }
    if (-not $exe) { throw '压缩包中没有找到 mihomo 可执行文件' }

    Copy-Item $exe.FullName $DestFile -Force
} finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}

Write-Host "✅ mihomo 已安装到 $DestFile"
& $DestFile -v
