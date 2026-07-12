[CmdletBinding()]
param(
    # 可覆盖的计划 artifact 路径。
    [string]$PlanPath = "",
    # 可覆盖的证据 artifact 根路径。
    [string]$EvidenceRoot = ""
)

$ErrorActionPreference = "Stop"

# 从脚本位置稳定定位后端模块与仓库根目录，不依赖调用方当前目录。
$scriptRoot = $PSScriptRoot
if ([string]::IsNullOrWhiteSpace($scriptRoot)) {
    $scriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
}
$backendRoot = [System.IO.Path]::GetFullPath((Join-Path $scriptRoot ".."))
$repositoryRoot = [System.IO.Path]::GetFullPath((Join-Path $backendRoot ".."))
if ([string]::IsNullOrWhiteSpace($PlanPath)) {
    $PlanPath = Join-Path $repositoryRoot ".omo/plans/go-service-architecture-foundation.md"
}
if ([string]::IsNullOrWhiteSpace($EvidenceRoot)) {
    $EvidenceRoot = Join-Path $repositoryRoot ".omo/evidence"
}

try {
    $env:PLAN_PATH = (Resolve-Path -LiteralPath $PlanPath).Path
    $env:EVIDENCE_ROOT = (Resolve-Path -LiteralPath $EvidenceRoot).Path
} catch {
    Write-Error "无法解析 QA 计划或证据 artifact 路径"
    exit 1
}

Push-Location $backendRoot
try {
    & go test -timeout 30s ./internal/reviewtest -run 'Test_PlanCompliance_' -count=1
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

    & git log --format=%s --all
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

    & git ls-tree -r --name-only HEAD
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

    Write-Output "F1 plan compliance: PASS"
} finally {
    Pop-Location
}
