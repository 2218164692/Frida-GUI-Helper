[CmdletBinding()]
param(
    [ValidateSet("amd64", "arm64")]
    [string]$Architecture = "amd64",

    [switch]$Clean
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$projectRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$bundleRoot = Join-Path $projectRoot "frida"
$installerRoot = Join-Path $projectRoot "build\windows\installer"
$payloadRoot = Join-Path $installerRoot "payload"
$payloadFrida = Join-Path $payloadRoot "frida"
$env:GOCACHE = Join-Path $projectRoot ".gocache"
$env:GOMODCACHE = Join-Path $projectRoot ".gomodcache"
$env:GOFLAGS = "-buildvcs=false"

function Assert-ChildPath {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Parent,

        [Parameter(Mandatory = $true)]
        [string]$Child
    )

    $resolvedParent = [System.IO.Path]::GetFullPath($Parent).TrimEnd("\", "/") + [System.IO.Path]::DirectorySeparatorChar
    $resolvedChild = [System.IO.Path]::GetFullPath($Child)
    if (-not $resolvedChild.StartsWith($resolvedParent, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "拒绝操作工作区之外的路径：$resolvedChild"
    }
}

function Resolve-WailsExecutable {
    $command = Get-Command "wails" -ErrorAction SilentlyContinue
    if ($null -ne $command) {
        return $command.Source
    }

    $repositoryCandidate = Join-Path $projectRoot ".gopath\bin\wails.exe"
    if (Test-Path -LiteralPath $repositoryCandidate -PathType Leaf) {
        return $repositoryCandidate
    }

    $goCommand = Get-Command "go" -ErrorAction SilentlyContinue
    if ($null -ne $goCommand) {
        $goPath = (& $goCommand.Source env GOPATH).Trim()
        if ($LASTEXITCODE -eq 0 -and -not [string]::IsNullOrWhiteSpace($goPath)) {
            $candidate = Join-Path $goPath "bin\wails.exe"
            if (Test-Path -LiteralPath $candidate -PathType Leaf) {
                return $candidate
            }
        }
    }

    throw "未找到 Wails CLI。请先运行：go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0"
}

function Enable-NSIS {
    $command = Get-Command "makensis" -ErrorAction SilentlyContinue
    if ($null -ne $command) {
        return $command.Source
    }

    $portableCandidate = Join-Path $projectRoot ".gopath\nsis-3.12\nsis-3.12\makensis.exe"
    if (Test-Path -LiteralPath $portableCandidate -PathType Leaf) {
        $env:PATH = "$(Split-Path -Parent $portableCandidate);$env:PATH"
        return $portableCandidate
    }

    $candidates = @()
    if (-not [string]::IsNullOrWhiteSpace(${env:ProgramFiles(x86)})) {
        $candidates += Join-Path ${env:ProgramFiles(x86)} "NSIS\makensis.exe"
    }
    if (-not [string]::IsNullOrWhiteSpace($env:ProgramFiles)) {
        $candidates += Join-Path $env:ProgramFiles "NSIS\makensis.exe"
    }

    foreach ($candidate in $candidates) {
        if (Test-Path -LiteralPath $candidate -PathType Leaf) {
            $env:PATH = "$(Split-Path -Parent $candidate);$env:PATH"
            return $candidate
        }
    }

    throw "未找到 NSIS（makensis.exe）。请先安装 NSIS，并将其加入 PATH。"
}

if (-not (Test-Path -LiteralPath $bundleRoot -PathType Container)) {
    throw "缺少内置 Frida 目录：$bundleRoot。请先生成完整的 frida/ 运行时。"
}

$bundleFiles = @(Get-ChildItem -LiteralPath $bundleRoot -File -Recurse -Force)
if ($bundleFiles.Count -eq 0) {
    throw "内置 Frida 目录为空：$bundleRoot。请先生成完整的 frida/ 运行时。"
}

$wailsExecutable = Resolve-WailsExecutable
$makensisExecutable = Enable-NSIS

Assert-ChildPath -Parent $installerRoot -Child $payloadFrida
if (Test-Path -LiteralPath $payloadFrida) {
    Remove-Item -LiteralPath $payloadFrida -Recurse -Force
}

New-Item -ItemType Directory -Path $payloadRoot -Force | Out-Null
Copy-Item -LiteralPath $bundleRoot -Destination $payloadRoot -Recurse -Force

$stagedFiles = @(Get-ChildItem -LiteralPath $payloadFrida -File -Recurse -Force)
if ($stagedFiles.Count -ne $bundleFiles.Count) {
    throw "Frida 安装负载复制不完整：源文件 $($bundleFiles.Count) 个，暂存文件 $($stagedFiles.Count) 个。"
}

$configPath = Join-Path $projectRoot "wails.json"
$config = Get-Content -LiteralPath $configPath -Raw | ConvertFrom-Json
$version = [string]$config.info.productVersion
if ([string]::IsNullOrWhiteSpace($version)) {
    throw "wails.json 未设置 info.productVersion。"
}

$arguments = @(
    "build"
    "-platform"
    "windows/$Architecture"
    "-nsis"
)
if ($Clean) {
    $arguments += "-clean"
}

Write-Host "Frida 负载：$($bundleFiles.Count) 个文件"
Write-Host "Wails CLI：$wailsExecutable"
Write-Host "NSIS：$makensisExecutable"
Write-Host "开始构建 Windows/$Architecture 安装包……"

Push-Location $projectRoot
try {
    & $wailsExecutable @arguments
    if ($LASTEXITCODE -ne 0) {
        throw "Wails/NSIS 构建失败，退出码：$LASTEXITCODE"
    }
}
finally {
    Pop-Location
}

$installerPath = Join-Path $projectRoot "build\bin\Frida-GUI-Helper-v$version-windows-$Architecture-setup.exe"
if (-not (Test-Path -LiteralPath $installerPath -PathType Leaf)) {
    throw "构建命令已结束，但没有找到预期安装包：$installerPath"
}

$hash = Get-FileHash -LiteralPath $installerPath -Algorithm SHA256
Write-Host "安装包构建完成：$installerPath"
Write-Host "SHA-256：$($hash.Hash)"
