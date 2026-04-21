#Requires -Version 5.1
[CmdletBinding()]
param(
	[string]$Prefix = (Join-Path $env:LOCALAPPDATA 'Programs\remindb'),
	[switch]$Help
)

$ErrorActionPreference = 'Stop'

function Show-Usage {
	@"
Usage: install.ps1 [-Prefix PATH]

Build and install the remindb binary.

Options:
  -Prefix PATH   Install root; binary is placed at PATH\bin\remindb.exe.
                 Default: %LOCALAPPDATA%\Programs\remindb
  -Help          Show this help.
"@
}

if ($Help) {
	Show-Usage
	exit 0
}

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
	Write-Error "'go' is not installed or not on PATH"
	exit 1
}

Set-Location -Path $PSScriptRoot

$binDir = Join-Path $Prefix 'bin'
$binPath = Join-Path $binDir 'remindb.exe'

New-Item -ItemType Directory -Force -Path $binDir | Out-Null

Write-Host "Building remindb -> $binPath"
& go build -o $binPath ./cmd/remindb
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "Installed: $binPath"

$onPath = ($env:PATH -split ';') | Where-Object { $_ -ieq $binDir }
if (-not $onPath) {
	Write-Host ""
	Write-Host "Note: $binDir is not on your PATH. Add it persistently with:"
	Write-Host "  [Environment]::SetEnvironmentVariable('Path', [Environment]::GetEnvironmentVariable('Path','User') + ';$binDir', 'User')"
}
