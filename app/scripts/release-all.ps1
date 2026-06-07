param(
  [string]$Version = "dev",
  [switch]$SkipChecks,
  [switch]$SkipAudit,
  [switch]$SkipWindows,
  [switch]$SkipLinux,
  [string]$UpdateRepository = "",
  [string]$WslDistro = ""
)

$ErrorActionPreference = "Stop"

function Convert-ToWslPath {
  param([string]$Path)

  $escapedPath = $Path -replace '\\', '/'
  if ($WslDistro) {
    $converted = & wsl.exe -d $WslDistro -- wslpath -a "$escapedPath"
  } else {
    $converted = & wsl.exe -- wslpath -a "$escapedPath"
  }

  if ($LASTEXITCODE -ne 0 -or -not $converted) {
    throw "Failed to convert path to WSL path: $Path"
  }

  return ($converted | Select-Object -First 1).Trim()
}

function Quote-Bash {
  param([string]$Value)
  return "'" + $Value.Replace("'", "'\''") + "'"
}

$ProjectRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$ReleaseDir = Join-Path $ProjectRoot "build\release"
New-Item -ItemType Directory -Force -Path $ReleaseDir | Out-Null

Set-Location $ProjectRoot

$artifacts = @()

if (-not $SkipWindows) {
  Write-Host "== Windows release =="
  $windowsArgs = @("-ExecutionPolicy", "Bypass", "-File", ".\scripts\release-local.ps1", "-Version", $Version)
  if ($UpdateRepository -ne "") {
    $windowsArgs += @("-UpdateRepository", $UpdateRepository)
  }
  if ($SkipChecks) {
    $windowsArgs += "-SkipChecks"
  }
  & powershell.exe @windowsArgs
  if ($LASTEXITCODE -ne 0) {
    throw "Windows release failed"
  }
  $artifacts += Join-Path $ReleaseDir "mimir-windows-amd64-$Version.zip"
}

if (-not $SkipLinux) {
  Write-Host "== Linux release via WSL =="
  $wslProjectRoot = Convert-ToWslPath $ProjectRoot
  $linuxArgs = @($Version)
  if ($SkipAudit) {
    $linuxArgs += "--skip-audit"
  }
  if ($UpdateRepository -ne "") {
    $linuxArgs += "--update-repo=$UpdateRepository"
  }
  $linuxCommand = "cd $(Quote-Bash $wslProjectRoot) && chmod +x ./scripts/release-linux.sh && ./scripts/release-linux.sh " + (($linuxArgs | ForEach-Object { Quote-Bash $_ }) -join " ")

  if ($WslDistro) {
    & wsl.exe -d $WslDistro -- bash -lc $linuxCommand
  } else {
    & wsl.exe -- bash -lc $linuxCommand
  }
  if ($LASTEXITCODE -ne 0) {
    throw "Linux release failed"
  }
  $artifacts += Join-Path $ReleaseDir "mimir-linux-amd64-$Version.tar.gz"
}

Write-Host ""
Write-Host "Release artifacts:"
foreach ($artifact in $artifacts) {
  Write-Host $artifact
}

Write-Host ""
Write-Host "macOS builds must run on macOS. On a Mac, run:"
if ($UpdateRepository -ne "") {
  Write-Host "  ./scripts/release-macos.sh $Version --update-repo=$UpdateRepository"
} else {
  Write-Host "  ./scripts/release-macos.sh $Version"
}
