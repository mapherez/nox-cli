# Native Windows installer handoff tests. All files and environment changes are disposable.
$ErrorActionPreference = 'Stop'
$repo = Split-Path $PSScriptRoot -Parent
$testRoot = Join-Path ([IO.Path]::GetTempPath()) ('nox-cli-installer-test-' + [guid]::NewGuid().ToString('N'))
$oldLocalAppData = $env:LOCALAPPDATA
$oldMarker = $env:NOX_CLI_TEST_ACTIVATED
$assets = Join-Path $testRoot 'assets'
$fixtureSource = Join-Path $testRoot 'main.go'
$fixtureDirectory = Join-Path $testRoot 'fixture'
$fixtureBinary = Join-Path $fixtureDirectory 'nox.exe'
try {
    [IO.Directory]::CreateDirectory($assets) | Out-Null
    [IO.Directory]::CreateDirectory($fixtureDirectory) | Out-Null
    $env:LOCALAPPDATA = Join-Path $testRoot 'user with spaces'
    $env:NOX_CLI_TEST_ACTIVATED = Join-Path $testRoot 'activation-marker'
    [IO.File]::WriteAllText($fixtureSource, @'
package main
import "os"
func main() {
 if len(os.Args)!=2 || os.Args[1]!="--complete-install" {os.Exit(2)}
 if os.WriteFile(os.Getenv("NOX_CLI_TEST_ACTIVATED"),[]byte("activated"),0600)!=nil {os.Exit(3)}
}
'@)
    & go build -o $fixtureBinary $fixtureSource
    if ($LASTEXITCODE -ne 0) { throw 'Unable to compile the installer handoff fixture.' }
    $architecture = [Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant()
    if ($architecture -eq 'x64') { $architecture = 'amd64' }
    $asset = "nox-cli_windows_${architecture}.zip"
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    [IO.Compression.ZipFile]::CreateFromDirectory($fixtureDirectory, (Join-Path $assets $asset))
    $digest = (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $assets $asset)).Hash
    [IO.File]::WriteAllText((Join-Path $assets 'checksums.txt'), "$digest  $asset`n")
    # Keep production URLs unchanged. Only the test session's download command is replaced.
    function Invoke-WebRequest {
        param([switch]$UseBasicParsing, [string]$Uri, [string]$OutFile)
        $prefix = 'https://github.com/mapherez/nox-cli/releases/latest/download/'
        if (-not $Uri.StartsWith($prefix)) { throw 'Unexpected installer source.' }
        Copy-Item -LiteralPath (Join-Path $assets $Uri.Substring($prefix.Length)) -Destination $OutFile
    }
    & (Join-Path $repo 'install.ps1')
    if ([IO.File]::ReadAllText($env:NOX_CLI_TEST_ACTIVATED) -ne 'activated') { throw 'Verified handoff was not executed.' }
    if (Test-Path -LiteralPath (Join-Path $env:LOCALAPPDATA 'nox-cli')) { throw 'Installer staging was not cleaned.' }
    Remove-Item -LiteralPath $env:NOX_CLI_TEST_ACTIVATED
    [IO.File]::WriteAllText((Join-Path $assets 'checksums.txt'), (('0' * 64) + "  $asset`n"))
    $failed = $false
    try { & (Join-Path $repo 'install.ps1') } catch { $failed = $true }
    if (-not $failed -or (Test-Path -LiteralPath $env:NOX_CLI_TEST_ACTIVATED)) { throw 'Unverified payload was executed.' }
    if (Test-Path -LiteralPath (Join-Path $env:LOCALAPPDATA 'nox-cli')) { throw 'Failure left installer staging.' }
    Write-Output 'PASS: Windows verified handoff, checksum rejection and staging cleanup. Activation/PATH logic is tested separately in Go.'
} finally {
    $env:LOCALAPPDATA = $oldLocalAppData
    $env:NOX_CLI_TEST_ACTIVATED = $oldMarker
    if (Test-Path -LiteralPath $testRoot) { Remove-Item -LiteralPath $testRoot -Recurse -Force }
}
