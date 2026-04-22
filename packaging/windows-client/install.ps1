$ErrorActionPreference = "Stop"

$InstallDir = Join-Path $env:LOCALAPPDATA "TermiChat\bin"
$ConfigDir = Join-Path $env:APPDATA "teamchat"
$BinaryPath = Join-Path $InstallDir "termichat.exe"

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
New-Item -ItemType Directory -Force -Path $ConfigDir | Out-Null

Copy-Item (Join-Path $PSScriptRoot "termichat.exe") $BinaryPath -Force

$config = @"
CHAT_SERVER_URL=https://termichat.zeraynce.com
CHAT_WORKSPACE=acme
CHAT_WORKSPACE_CODE=acme123
CHAT_DEFAULT_CHANNEL=lobby
"@
Set-Content -Path (Join-Path $ConfigDir "client.env") -Value $config -NoNewline

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (-not $userPath) {
  $userPath = ""
}
if (($userPath -split ";") -notcontains $InstallDir) {
  $newPath = if ([string]::IsNullOrWhiteSpace($userPath)) { $InstallDir } else { "$userPath;$InstallDir" }
  [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
}

Write-Host ""
Write-Host "Installed termichat to $BinaryPath"
Write-Host "Saved config to $(Join-Path $ConfigDir 'client.env')"
Write-Host "Open a new Windows Terminal session, then run:"
Write-Host "  termichat"
