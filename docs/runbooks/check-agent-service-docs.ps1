[CmdletBinding()]
param(
    [Parameter()]
    [string]$RepositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
)

$ErrorActionPreference = "Stop"

$requiredFiles = @(
    "docs\agent-streaming-architecture.md",
    "docs\agent-service-development.md",
    "docs\runbooks\agent-service.md",
    "Taskfile.yml",
    "agent\compose.yaml",
    "agent\.env.example",
    "agent\scripts\qa_agent.py",
    ".omo\evidence\scyg-agent-service\T34\review\adversarial-verify.json"
)

foreach ($relativePath in $requiredFiles) {
    $path = Join-Path $RepositoryRoot $relativePath
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "T35 documentation check failed: missing path $relativePath"
    }
}

$utf8 = [System.Text.Encoding]::UTF8
$architecture = [System.IO.File]::ReadAllText((Join-Path $RepositoryRoot "docs\agent-streaming-architecture.md"), $utf8)
$development = [System.IO.File]::ReadAllText((Join-Path $RepositoryRoot "docs\agent-service-development.md"), $utf8)
$runbook = [System.IO.File]::ReadAllText((Join-Path $RepositoryRoot "docs\runbooks\agent-service.md"), $utf8)
$allDocumentation = "$architecture`n$development`n$runbook"

$requiredFragments = @(
    "task qa:agent:static",
    "task qa:agent",
    "uv run --locked",
    "/api/runs/{run_id}/events",
    "?cursor=<",
    "Last-Event-ID",
    "Last-Event-ID == cursor",
    "seq > cursor",
    "run_succeeded",
    "run_failed",
    "run_cancelled",
    "Idempotency-Replayed=false",
    "Idempotency-Replayed=true",
    "SIMPLE 4",
    "DEEP",
    "scyg_agent",
    "blocked_by_environment",
    "S1",
    "S2",
    "S3",
    "T22",
    "T31",
    "JWT"
)

foreach ($fragment in $requiredFragments) {
    if (-not $allDocumentation.Contains($fragment)) {
        throw "T35 documentation check failed: missing contract fragment $fragment"
    }
}

$forbiddenFragments = @(
    "?after=",
    "run.completed",
    "task qa:agent`n# exit 0"
)

foreach ($fragment in $forbiddenFragments) {
    if ($allDocumentation.Contains($fragment)) {
        throw "T35 documentation check failed: stale or out-of-scope content $fragment"
    }
}

function Test-AllowedTokenUrlPolicy([string]$line) {
    return $line -match "(?i)\b(jwt|bearer|authorization|token)\b.{0,32}\b(must not|cannot|never)\b.{0,32}\b(url|query|query string|sse query)\b|\b(jwt|bearer|authorization|token)\b.{0,32}(\u4E0D\u5F97|\u7EDD\u4E0D\u80FD|\u4E0D\u80FD).{0,32}\b(url|query|query string|sse query)\b"
}

foreach ($line in ($allDocumentation -split "\r?\n")) {
    if ($line -match "(?i)\bS[4-7]\b.{0,64}\b(pass|passed|passing|complete|completed|success|succeeded)\b|\bS[4-7]\b.{0,64}(\u5DF2\u901A\u8FC7|\u901A\u8FC7|\u5DF2\u5B8C\u6210)") {
        throw "T35 documentation check failed: deferred scenario success claim $line"
    }
    if ($line -match "(?i)(\b(jwt|bearer|authorization|token)\b.{0,64}\b(url|query|query string|sse query)\b|\b(url|query|query string|sse query)\b.{0,64}\b(jwt|bearer|authorization|token)\b)") {
        if (-not (Test-AllowedTokenUrlPolicy $line)) {
            throw "T35 documentation check failed: token URL guidance $line"
        }
    }
    if ($line -match "(?i)\b(T34|S[1-3])\b.{0,64}\b(dynamic\s+)?(e2e\s+)?pass(ed)?\b") {
        throw "T35 documentation check failed: dynamic T34 pass claim $line"
    }
}

foreach ($documentPath in @(
    (Join-Path $RepositoryRoot "docs\agent-streaming-architecture.md"),
    (Join-Path $RepositoryRoot "docs\agent-service-development.md"),
    (Join-Path $RepositoryRoot "docs\runbooks\agent-service.md")
)) {
    $document = [System.IO.File]::ReadAllText($documentPath, $utf8)
    $markdownLinks = [regex]::Matches($document, "\]\(([^)#]+)(?:#[^)]*)?\)")
    foreach ($match in $markdownLinks) {
        $target = $match.Groups[1].Value
        if ($target -match "^[a-z]+:") {
            continue
        }
        $targetPath = Join-Path (Split-Path -Parent $documentPath) $target
        if (-not (Test-Path -LiteralPath $targetPath -PathType Leaf)) {
            throw "T35 documentation check failed: stale Markdown link $target"
        }
    }
}

$taskfile = Get-Content -Raw -LiteralPath (Join-Path $RepositoryRoot "Taskfile.yml")
if (-not $taskfile.Contains("uv run --locked --project agent python agent/scripts/qa_agent.py --mode static")) {
    throw "T35 documentation check failed: static QA command diverges from Taskfile"
}
if (-not $taskfile.Contains("uv run --locked --project agent python agent/scripts/qa_agent.py --mode full")) {
    throw "T35 documentation check failed: full QA command diverges from Taskfile"
}

$qaScript = Get-Content -Raw -LiteralPath (Join-Path $RepositoryRoot "agent\scripts\qa_agent.py")
if (-not $qaScript.Contains("T32 topology prerequisites: missing tool(s):")) {
    throw "T35 documentation check failed: full QA fail-closed preflight is absent"
}

$t34EvidenceRoot = Join-Path $RepositoryRoot ".omo\evidence\scyg-agent-service\T34"
$t34Review = Get-Content -Raw -LiteralPath (Join-Path $t34EvidenceRoot "review\adversarial-verify.json") | ConvertFrom-Json
if ($t34Review.dynamic_execution.status -ne "blocked_by_environment") {
    throw "T35 documentation check failed: T34 dynamic state is not blocked_by_environment"
}
if (
    $t34Review.dynamic_execution.claimed_pass -ne $false -or
    $t34Review.dynamic_execution.pass_markers_emitted -ne $false -or
    $t34Review.dynamic_execution.resources_started -ne $false
) {
    throw "T35 documentation check failed: T34 dynamic pass claim is present"
}
if (@(Get-ChildItem -LiteralPath $t34EvidenceRoot -Filter "receipt.json" -File -Recurse).Count -ne 0) {
    throw "T35 documentation check failed: T34 dynamic receipt is present"
}

Write-Output "T35 agent documentation: PASS"
