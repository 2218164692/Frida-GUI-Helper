$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$expected = (Get-Content -Raw (Join-Path $root "VERSION")).Trim()
$package = Get-Content -Raw (Join-Path $root "frontend/package.json") | ConvertFrom-Json
$wails = Get-Content -Raw (Join-Path $root "wails.json") | ConvertFrom-Json

$versions = [ordered]@{
    "VERSION" = $expected
    "frontend/package.json" = [string]$package.version
    "wails.json" = [string]$wails.info.productVersion
}

$mismatches = $versions.GetEnumerator() | Where-Object { $_.Value -ne $expected }
if ($mismatches) {
    $details = ($mismatches | ForEach-Object { "$($_.Key)=$($_.Value)" }) -join ", "
    throw "Version mismatch. Expected $expected; $details"
}

Write-Host "Version metadata is consistent: $expected"
