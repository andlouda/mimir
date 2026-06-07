param(
  [string]$Version = "dev",
  [string]$UpdateRepository = "",
  [switch]$SkipChecks
)

$ErrorActionPreference = "Stop"

$ProjectRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$ReleaseDir = Join-Path $ProjectRoot "build\release"
$ArtifactName = "mimir-windows-amd64-$Version.zip"
$ArtifactPath = Join-Path $ReleaseDir $ArtifactName

Set-Location $ProjectRoot
New-Item -ItemType Directory -Force -Path $ReleaseDir | Out-Null

$GoPackages = Get-ChildItem -Recurse -Filter *.go |
  Where-Object { $_.FullName -notmatch '\\frontend\\' -and $_.FullName -notmatch '\\build\\' } |
  ForEach-Object { Resolve-Path -Relative $_.DirectoryName } |
  Sort-Object -Unique

# Build the frontend first (always) so go:embed (all:frontend/dist) and the Wails
# build both find frontend/dist — independent of -SkipChecks.
Write-Host "== Frontend install =="
Push-Location frontend
npm ci

Write-Host "== Frontend build =="
npm run build
Pop-Location

if (-not $SkipChecks) {
  Write-Host "== Go tests =="
  go test @GoPackages

  Write-Host "== Go race tests =="
  go test -race @GoPackages

  Write-Host "== npm audit =="
  Push-Location frontend
  npm audit --audit-level=moderate
  Pop-Location

  if (Get-Command govulncheck -ErrorAction SilentlyContinue) {
    Write-Host "== govulncheck =="
    govulncheck @GoPackages
  } else {
    Write-Host "Skipping govulncheck; install with: go install golang.org/x/vuln/cmd/govulncheck@latest"
  }
}

Write-Host "== Wails build =="
$Ldflags = "-X main.AppVersion=$Version"
if ($UpdateRepository -ne "") {
  $Ldflags = "$Ldflags -X main.UpdateRepository=$UpdateRepository"
}
# -s: skip Wails' own frontend build; frontend/dist is already built above.
wails build -s -ldflags $Ldflags

$BinaryPath = Join-Path $ProjectRoot "build\bin\mimir.exe"
if (-not (Test-Path $BinaryPath)) {
  throw "Expected Windows binary not found: $BinaryPath"
}

if (Test-Path $ArtifactPath) {
  Remove-Item $ArtifactPath
}

Write-Host "== Packaging $ArtifactName =="
Compress-Archive -Path $BinaryPath -DestinationPath $ArtifactPath -Force

Write-Host "== Checksums =="
Push-Location $ReleaseDir
if (Test-Path "checksums.txt") {
  $existing = Get-Content "checksums.txt" | Where-Object { $_ -notmatch "\s\s$([regex]::Escape($ArtifactName))$" }
  $existing | Out-File -FilePath "checksums.txt" -Encoding ascii
}
Get-FileHash -Algorithm SHA256 $ArtifactName | ForEach-Object {
  "$($_.Hash.ToLower())  $ArtifactName" | Out-File -FilePath "checksums.txt" -Encoding ascii -Append
}
Pop-Location

Write-Host "Release artifact:"
Write-Host $ArtifactPath
