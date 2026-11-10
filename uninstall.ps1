# Removal uses a new verified release payload, never the installed executable.
param([switch]$Purge)
$ErrorActionPreference = 'Stop'
$source = Invoke-RestMethod -Uri 'https://github.com/mapherez/nox-cli/releases/latest/download/install.ps1' -TimeoutSec 60
& ([scriptblock]::Create($source)) -Action Uninstall -Purge:$Purge
