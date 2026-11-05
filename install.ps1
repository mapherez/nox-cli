# Download/verify the official Windows payload; native code owns activation.
$ErrorActionPreference = 'Stop'
if (-not $env:LOCALAPPDATA) { throw 'LOCALAPPDATA is unavailable.' }
$root = Join-Path $env:LOCALAPPDATA 'nox-cli'
$stage = Join-Path $root 'install'
$payload = Join-Path $stage 'payload.zip'
$checksums = Join-Path $stage 'checksums.txt'
$executable = Join-Path $stage 'nox.exe'
$lockPath = Join-Path $stage 'installer.lock'
$lock = $null
foreach ($directory in @($root, $stage)) {
    if ((Test-Path -LiteralPath $directory) -and ((Get-Item -LiteralPath $directory).Attributes -band [IO.FileAttributes]::ReparsePoint)) {
        throw 'Refusing a linked installation directory.'
    }
}
[IO.Directory]::CreateDirectory($stage) | Out-Null
if ((Test-Path -LiteralPath $lockPath) -and ((Get-Item -LiteralPath $lockPath).Attributes -band [IO.FileAttributes]::ReparsePoint)) {
    throw 'Refusing a linked installer lock.'
}
try {
    # An exclusive handle distinguishes a live installer from a stale/empty file.
    $lock = [IO.File]::Open($lockPath, [IO.FileMode]::OpenOrCreate, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
    $lock.SetLength(0)
    $pidBytes = [Text.Encoding]::UTF8.GetBytes([string]$PID)
    $lock.Write($pidBytes, 0, $pidBytes.Length)
    $lock.Flush()
    foreach ($file in @($payload, $checksums, $executable)) {
        if (Test-Path -LiteralPath $file) { Remove-Item -LiteralPath $file }
    }
    Get-ChildItem -LiteralPath $stage -Filter 'activate-*' -File | Remove-Item
    $architecture = [Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant()
    switch ($architecture) { 'x64' { $architecture = 'amd64' } 'arm64' { } default { throw 'Unsupported CPU architecture.' } }
    $asset = "nox-cli_windows_${architecture}.zip"
    $release = 'https://github.com/mapherez/nox-cli/releases/latest/download'
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    Invoke-WebRequest -UseBasicParsing -Uri "$release/$asset" -OutFile $payload
    Invoke-WebRequest -UseBasicParsing -Uri "$release/checksums.txt" -OutFile $checksums
    $matchesForAsset = @([IO.File]::ReadAllLines($checksums) | Where-Object { $_ -match ('^[a-fA-F0-9]{64}\s+\*?' + [regex]::Escape($asset) + '$') })
    if ($matchesForAsset.Count -ne 1) { throw 'Invalid release checksum.' }
    $expected = ($matchesForAsset[0] -split '\s+')[0]
    $actual = (Get-FileHash -LiteralPath $payload -Algorithm SHA256).Hash
    if ($actual -ne $expected) { throw 'Release checksum verification failed.' }
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $archive = [IO.Compression.ZipFile]::OpenRead($payload)
    try {
        if ($archive.Entries.Count -ne 1 -or $archive.Entries[0].FullName -ne 'nox.exe') { throw 'Unexpected release archive contents.' }
        $entry = $archive.Entries[0].Open()
        try {
            $target = [IO.File]::Open($executable, [IO.FileMode]::CreateNew)
            try { $entry.CopyTo($target) } finally { $target.Dispose() }
        } finally { $entry.Dispose() }
    } finally { $archive.Dispose() }
    & $executable --complete-install
    if ($LASTEXITCODE -ne 0) { throw 'Native installation did not complete.' }
} catch {
    throw 'NoX CLI installation failed. Check release availability, connectivity, and whether another nox command is installed.'
} finally {
    if ($null -ne $lock) {
        $lock.Dispose()
        foreach ($file in @($payload, $checksums, $executable)) {
            if (Test-Path -LiteralPath $file) { Remove-Item -LiteralPath $file -ErrorAction SilentlyContinue }
        }
        Get-ChildItem -LiteralPath $stage -Filter 'activate-*' -File -ErrorAction SilentlyContinue | Remove-Item -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $lockPath -ErrorAction SilentlyContinue
        foreach ($directory in @($stage, $root)) {
            if ((Test-Path -LiteralPath $directory) -and ([IO.Directory]::GetFileSystemEntries($directory).Length -eq 0)) {
                [IO.Directory]::Delete($directory)
            }
        }
    }
}
