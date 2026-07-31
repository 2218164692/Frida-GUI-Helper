[CmdletBinding()]
param(
    [string]$Version = "latest",
    [string]$OutputDirectory = ""
)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot
if ([string]::IsNullOrWhiteSpace($OutputDirectory)) {
    $OutputDirectory = Join-Path $repoRoot "frida"
}
$bundleRoot = [System.IO.Path]::GetFullPath($OutputDirectory)
$repoFull = [System.IO.Path]::GetFullPath($repoRoot)
if (-not $bundleRoot.StartsWith($repoFull, [System.StringComparison]::OrdinalIgnoreCase)) {
    throw "OutputDirectory must stay inside the repository: $bundleRoot"
}

$env:GOCACHE = Join-Path $repoRoot ".gocache"
$env:GOMODCACHE = Join-Path $repoRoot ".gomodcache"
$env:GOFLAGS = "-buildvcs=false"

Push-Location $repoRoot
try {
    & go run ./cmd/frida-bundle -output $bundleRoot -version $Version
    if ($LASTEXITCODE -ne 0) {
        throw "Frida bundle preparation failed with exit code $LASTEXITCODE"
    }

    $launcher = Join-Path $bundleRoot "frida-launcher.exe"
    & go build -trimpath -ldflags "-s -w" -o $launcher ./cmd/frida-launcher
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $launcher -PathType Leaf)) {
        throw "Unable to build the relocatable Frida launcher"
    }
    foreach ($name in @("frida.exe", "frida-ps.exe", "frida-ls-devices.exe")) {
        Copy-Item -LiteralPath $launcher -Destination (Join-Path $bundleRoot $name) -Force
    }
    Remove-Item -LiteralPath $launcher -Force
    Copy-Item -LiteralPath (Join-Path $repoRoot "tools\runtime\launcher.py") -Destination (Join-Path $bundleRoot "launcher.py") -Force

    $fridaExe = Join-Path $bundleRoot "frida.exe"
    $actualVersion = (& $fridaExe --version | Select-Object -First 1).Trim()
    if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($actualVersion)) {
        throw "Bundled frida.exe smoke test failed"
    }
    $manifest = Get-Content -LiteralPath (Join-Path $bundleRoot "manifest.json") -Raw | ConvertFrom-Json
    if ($actualVersion.TrimStart("v") -ne ([string]$manifest.version).TrimStart("v")) {
        throw "Frida version mismatch: launcher=$actualVersion manifest=$($manifest.version)"
    }
    Write-Host "Frida $actualVersion bundle ready: $bundleRoot"
}
finally {
    Pop-Location
}
