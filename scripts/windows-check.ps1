$StrictAudit = $false
if ($args -contains "-StrictAudit") {
  $StrictAudit = $true
}

$ErrorActionPreference = "Stop"

Set-Location (Join-Path $PSScriptRoot "..")

$GoPackages = Get-ChildItem -Recurse -Filter *.go |
  Where-Object { $_.FullName -notmatch '\\frontend\\' -and $_.FullName -notmatch '\\build\\' } |
  ForEach-Object { Resolve-Path -Relative $_.DirectoryName } |
  Sort-Object -Unique

Write-Host "== Go version =="
go version

Write-Host "== Go tests =="
go test @GoPackages

Write-Host "== Go race tests =="
go test -race @GoPackages

Write-Host "== Frontend install =="
Push-Location frontend
npm ci

Write-Host "== Frontend build =="
npm run build
Pop-Location

Write-Host "== Wails build =="
wails build

if (Get-Command govulncheck -ErrorAction SilentlyContinue) {
  Write-Host "== govulncheck =="
  govulncheck @GoPackages
} else {
  Write-Host "Skipping govulncheck; install with: go install golang.org/x/vuln/cmd/govulncheck@latest"
}

Push-Location frontend
Write-Host "== npm audit =="
npm audit --audit-level=moderate
if ($LASTEXITCODE -ne 0 -and -not $StrictAudit) {
  Write-Host "npm audit reported findings. Re-run with -StrictAudit to fail this script on audit findings."
  $global:LASTEXITCODE = 0
}
Pop-Location
