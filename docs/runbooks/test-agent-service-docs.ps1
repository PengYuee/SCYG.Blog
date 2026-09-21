[CmdletBinding()]
param(
    [Parameter()]
    [string]$RepositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
)

$ErrorActionPreference = "Stop"
$fixtureRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("scyg-t35-docs-" + [guid]::NewGuid())

function New-Fixture {
    $root = Join-Path $fixtureRoot ([guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Path $root | Out-Null
    New-Item -ItemType Directory -Path (Join-Path $root "docs") | Out-Null
    Copy-Item -Recurse -Force (Join-Path $RepositoryRoot "docs\*") (Join-Path $root "docs")
    Copy-Item -Force (Join-Path $RepositoryRoot "Taskfile.yml") (Join-Path $root "Taskfile.yml")
    New-Item -ItemType Directory -Force -Path (Join-Path $root "agent\scripts") | Out-Null
    Copy-Item -Force (Join-Path $RepositoryRoot "agent\compose.yaml") (Join-Path $root "agent\compose.yaml")
    Copy-Item -Force (Join-Path $RepositoryRoot "agent\.env.example") (Join-Path $root "agent\.env.example")
    Copy-Item -Force (Join-Path $RepositoryRoot "agent\scripts\qa_agent.py") (Join-Path $root "agent\scripts\qa_agent.py")
    $reviewDirectory = Join-Path $root ".omo\evidence\scyg-agent-service\T34\review"
    New-Item -ItemType Directory -Force -Path $reviewDirectory | Out-Null
    Copy-Item -Force (Join-Path $RepositoryRoot ".omo\evidence\scyg-agent-service\T34\review\adversarial-verify.json") (Join-Path $reviewDirectory "adversarial-verify.json")
    return $root
}

function Invoke-PassingFixture([string]$name, [scriptblock]$mutate) {
    $root = New-Fixture
    try {
        & $mutate $root
        & (Join-Path $root "docs\runbooks\check-agent-service-docs.ps1") -RepositoryRoot $root | Out-Null
        Write-Output "PASS $name"
    }
    finally {
        Remove-Item -Recurse -Force $root
    }
}

function Invoke-FailingFixture([string]$name, [string]$expected, [scriptblock]$mutate) {
    $root = New-Fixture
    try {
        & $mutate $root
        $failed = $false
        try {
            & (Join-Path $root "docs\runbooks\check-agent-service-docs.ps1") -RepositoryRoot $root | Out-Null
        }
        catch {
            $failed = $true
            if ($_.Exception.Message -notlike "*$expected*") {
                throw "fixture $name failed with unexpected message: $($_.Exception.Message)"
            }
        }
        if (-not $failed) {
            throw "fixture $name unexpectedly passed"
        }
        Write-Output "PASS $name"
    }
    finally {
        Remove-Item -Recurse -Force $root
    }
}

try {
    New-Item -ItemType Directory -Path $fixtureRoot | Out-Null
    Invoke-PassingFixture "valid-local-link" {
        param($root)
        $fixtures = Join-Path $root "docs\fixtures"
        New-Item -ItemType Directory -Path $fixtures | Out-Null
        Set-Content -NoNewline -Encoding utf8 -LiteralPath (Join-Path $fixtures "valid.md") -Value "valid"
        Add-Content -Encoding utf8 -LiteralPath (Join-Path $root "docs\agent-service-development.md") -Value "`n[valid](fixtures/valid.md)"
    }
    Invoke-FailingFixture "stale-local-link" "stale Markdown link" {
        param($root)
        Add-Content -Encoding utf8 -LiteralPath (Join-Path $root "docs\agent-service-development.md") -Value "`n[missing](fixtures/missing.md)"
    }
    Invoke-FailingFixture "stale-cursor" "stale or out-of-scope" {
        param($root)
        Add-Content -Encoding utf8 -LiteralPath (Join-Path $root "docs\agent-service-development.md") -Value "`n?after=7"
    }
    Invoke-FailingFixture "stale-event" "stale or out-of-scope" {
        param($root)
        Add-Content -Encoding utf8 -LiteralPath (Join-Path $root "docs\agent-service-development.md") -Value "`nrun.completed"
    }
    Invoke-FailingFixture "s4-pass" "deferred scenario success claim" {
        param($root)
        Add-Content -Encoding utf8 -LiteralPath (Join-Path $root "docs\agent-service-development.md") -Value "`nS4 PASS"
    }
    Invoke-FailingFixture "s4-pass-with-negation" "deferred scenario success claim" {
        param($root)
        Add-Content -Encoding utf8 -LiteralPath (Join-Path $root "docs\agent-service-development.md") -Value "`nS4 PASS; this line is not a claim."
    }
    Invoke-FailingFixture "s5-chinese-pass" "deferred scenario success claim" {
        param($root)
        $claim = "S5 " + [char]0x5DF2 + [char]0x901A + [char]0x8FC7
        Add-Content -Encoding utf8 -LiteralPath (Join-Path $root "docs\agent-service-development.md") -Value "`n$claim"
    }
    Invoke-FailingFixture "s6-dynamic-pass" "deferred scenario success claim" {
        param($root)
        Add-Content -Encoding utf8 -LiteralPath (Join-Path $root "docs\agent-service-development.md") -Value "`nS6 dynamic PASS"
    }
    Invoke-FailingFixture "s7-chinese-pass" "deferred scenario success claim" {
        param($root)
        $claim = "S7 " + [char]0x901A + [char]0x8FC7
        Add-Content -Encoding utf8 -LiteralPath (Join-Path $root "docs\agent-service-development.md") -Value "`n$claim"
    }
    Invoke-FailingFixture "s7-lowercase-pass" "deferred scenario success claim" {
        param($root)
        Add-Content -Encoding utf8 -LiteralPath (Join-Path $root "docs\agent-service-development.md") -Value "`nS7 dynamic pass"
    }
    Invoke-FailingFixture "token-url" "token URL guidance" {
        param($root)
        Add-Content -Encoding utf8 -LiteralPath (Join-Path $root "docs\agent-service-development.md") -Value "`nSend Bearer token in URL query string."
    }
    Invoke-FailingFixture "token-url-with-negation" "token URL guidance" {
        param($root)
        Add-Content -Encoding utf8 -LiteralPath (Join-Path $root "docs\agent-service-development.md") -Value "`nDo not log secrets. Send Bearer token in URL query string."
    }
    Invoke-FailingFixture "dynamic-t34-pass" "dynamic T34 pass claim" {
        param($root)
        Add-Content -Encoding utf8 -LiteralPath (Join-Path $root "docs\agent-service-development.md") -Value "`nT34 dynamic E2E PASS"
    }
    Invoke-FailingFixture "dynamic-t34-state" "T34 dynamic state" {
        param($root)
        $path = Join-Path $root ".omo\evidence\scyg-agent-service\T34\review\adversarial-verify.json"
        $json = Get-Content -Raw -LiteralPath $path
        Set-Content -NoNewline -Encoding utf8 -LiteralPath $path -Value $json.Replace("blocked_by_environment", "passed")
    }
    Invoke-FailingFixture "dynamic-t34-claimed-pass" "T34 dynamic pass claim" {
        param($root)
        $path = Join-Path $root ".omo\evidence\scyg-agent-service\T34\review\adversarial-verify.json"
        $json = Get-Content -Raw -LiteralPath $path
        Set-Content -NoNewline -Encoding utf8 -LiteralPath $path -Value $json.Replace('"claimed_pass": false', '"claimed_pass": true')
    }
    Invoke-FailingFixture "dynamic-t34-pass-marker" "T34 dynamic pass claim" {
        param($root)
        $path = Join-Path $root ".omo\evidence\scyg-agent-service\T34\review\adversarial-verify.json"
        $json = Get-Content -Raw -LiteralPath $path
        Set-Content -NoNewline -Encoding utf8 -LiteralPath $path -Value $json.Replace('"pass_markers_emitted": false', '"pass_markers_emitted": true')
    }
    Invoke-FailingFixture "dynamic-t34-resources" "T34 dynamic pass claim" {
        param($root)
        $path = Join-Path $root ".omo\evidence\scyg-agent-service\T34\review\adversarial-verify.json"
        $json = Get-Content -Raw -LiteralPath $path
        Set-Content -NoNewline -Encoding utf8 -LiteralPath $path -Value $json.Replace('"resources_started": false', '"resources_started": true')
    }
    Invoke-FailingFixture "dynamic-t34-receipt" "T34 dynamic receipt" {
        param($root)
        $path = Join-Path $root ".omo\evidence\scyg-agent-service\T34\receipt.json"
        Set-Content -NoNewline -Encoding utf8 -LiteralPath $path -Value "{}"
    }
    Write-Output "T35 fixture matrix: PASS"
}
finally {
    if (Test-Path -LiteralPath $fixtureRoot) {
        Remove-Item -Recurse -Force $fixtureRoot
    }
}
