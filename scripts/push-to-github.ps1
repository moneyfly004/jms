# 把本地这次改动安全地推送到 https://github.com/moneyfly004/jms
#
# 背景：本地仓库是在没有 .git 的目录里重新 git init 出来的，
# 与 GitHub 上的历史没有共同祖先，直接 git push 会被拒绝。
# 这个脚本会先 fetch 远程历史，再把本地改动“挂”到远程分支之上，
# 生成一个可以 fast-forward 的提交，不会丢失 GitHub 上的任何历史。
#
# 用法:
#   pwsh -File .\scripts\push-to-github.ps1
#   pwsh -File .\scripts\push-to-github.ps1 -WhatIfOnly   # 只看会做什么，不做任何修改

param(
    [switch]$WhatIfOnly,
    [string]$Remote = 'origin'
)

$PSNativeCommandUseErrorActionPreference = $false
$ErrorActionPreference = 'Stop'

function Invoke-Git {
    param([Parameter(ValueFromRemainingArguments = $true)][string[]]$Args)
    & git @Args
    return $LASTEXITCODE
}

# --- 前置检查 ---------------------------------------------------------------
if ((& git rev-parse --is-inside-work-tree 2>$null) -ne 'true') {
    throw '当前目录不是 git 仓库，请在项目根目录运行本脚本。'
}

if (-not (& git remote get-url $Remote 2>$null)) {
    Write-Host "未找到远程 $Remote，正在添加 https://github.com/moneyfly004/jms.git"
    if ((Invoke-Git remote add $Remote 'https://github.com/moneyfly004/jms.git') -ne 0) {
        throw "添加远程 $Remote 失败"
    }
}

$branch = (& git rev-parse --abbrev-ref HEAD).Trim()
$commitMessage = (& git log -1 --pretty=%B 2>$null)
if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($commitMessage)) {
    throw '本地没有任何提交，请先提交改动。'
}
$commitMessage = $commitMessage.Trim()

Write-Host "本地分支      : $branch"
Write-Host "远程地址      : $((& git remote get-url $Remote).Trim())"
Write-Host "待推送的提交  : $((& git log --oneline -1).Trim())"

# --- 获取远程历史 -----------------------------------------------------------
Write-Host ''
Write-Host '正在执行 git fetch（私有仓库会要求输入 GitHub 凭据）...'
if ((Invoke-Git fetch $Remote) -ne 0) {
    throw "git fetch 失败：请确认已登录 GitHub，并且对 moneyfly004/jms 有写入权限。"
}
Invoke-Git remote set-head $Remote -a | Out-Null

$remoteRef = $null
$candidate = & git symbolic-ref --short "refs/remotes/$Remote/HEAD" 2>$null
if ($LASTEXITCODE -eq 0 -and $candidate) {
    $remoteRef = $candidate.Trim()
}
if (-not $remoteRef) {
    foreach ($name in @('main', 'master')) {
        & git show-ref --verify --quiet "refs/remotes/$Remote/$name"
        if ($LASTEXITCODE -eq 0) { $remoteRef = "$Remote/$name"; break }
    }
}

$remoteBranch = if ($remoteRef) { $remoteRef -replace "^$Remote/", '' } else { $null }

if ($WhatIfOnly) {
    Write-Host ''
    if ($remoteRef) {
        Write-Host "[dry-run] 远程已有历史 $remoteRef，将执行："
        Write-Host "  git branch -m $remoteBranch        # 如分支名不同"
        Write-Host "  git reset --soft $remoteRef"
        Write-Host "  git commit -F <本次提交说明>"
        Write-Host "  git push -u $Remote HEAD:$remoteBranch"
    } else {
        Write-Host "[dry-run] 远程仓库为空，将执行："
        Write-Host "  git push -u $Remote $branch"
    }
    exit 0
}

# --- 远程为空：直接推送 -----------------------------------------------------
if (-not $remoteRef) {
    Write-Host ''
    Write-Host "远程仓库没有分支，直接推送 $branch ..."
    if ((Invoke-Git push -u $Remote $branch) -ne 0) { throw 'git push 失败' }
    Write-Host '✅ 推送完成'
    exit 0
}

# --- 远程有历史：把本地改动挂到远程分支之上 ---------------------------------
Write-Host ''
Write-Host "检测到远程历史 $remoteRef，把本地改动挂到它之上（不会丢失远程历史）..."

if ($remoteBranch -ne $branch) {
    Write-Host "  本地分支 $branch 重命名为 $remoteBranch 以对齐远程"
    if ((Invoke-Git branch -m $remoteBranch) -ne 0) { throw '重命名本地分支失败' }
    $branch = $remoteBranch
}

# --soft 只移动 HEAD，索引（= 本次改动的完整快照）保持不变
if ((Invoke-Git reset --soft $remoteRef) -ne 0) { throw "git reset --soft $remoteRef 失败" }

$staged = @(& git diff --staged --name-status)
if ($staged.Count -eq 0) {
    Write-Host '本地内容与远程完全一致，无需提交。'
    exit 0
}

Write-Host ''
Write-Host '本次相对远程的改动：'
$staged | ForEach-Object { Write-Host "  $_" }
Write-Host ''

$tmpMsg = Join-Path ([System.IO.Path]::GetTempPath()) ("jms-commit-" + [guid]::NewGuid().ToString('N') + '.txt')
try {
    Set-Content -Path $tmpMsg -Value $commitMessage -Encoding utf8
    if ((Invoke-Git commit -F $tmpMsg) -ne 0) { throw 'git commit 失败' }
} finally {
    Remove-Item -Force $tmpMsg -ErrorAction SilentlyContinue
}

if ((Invoke-Git push -u $Remote "HEAD:$remoteBranch") -ne 0) {
    throw 'git push 失败（可能是分支保护规则或权限不足）'
}

Write-Host ''
Write-Host "✅ 已推送到 $Remote/$remoteBranch"
Write-Host '接下来到 GitHub 的 Actions 页面确认工作流已出现，如有提示请点击 Enable Actions。'
