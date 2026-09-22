param(
    [string]$Repo,
    [Parameter(Mandatory=$true)][ValidatePattern('^[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9]+([.-][A-Za-z0-9]+)*)?$')][string]$Version,
    [string]$InstallDir = (Join-Path $env:LOCALAPPDATA 'Klib\bin'),
    [string]$SourceDir
)
$ErrorActionPreference = 'Stop'
if ($env:OS -ne 'Windows_NT' -or ($env:PROCESSOR_ARCHITECTURE -ne 'AMD64' -and $env:PROCESSOR_ARCHITEW6432 -ne 'AMD64')) {
    throw 'This installer supports Windows x64 only.'
}
$asset = "klib_${Version}_windows-amd64.exe"
$tempDir = Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid().ToString('N'))
$staged = $null
New-Item -ItemType Directory -Path $tempDir | Out-Null
try {
    if ($SourceDir) {
        Copy-Item -LiteralPath (Join-Path $SourceDir $asset) -Destination (Join-Path $tempDir $asset)
        Copy-Item -LiteralPath (Join-Path $SourceDir 'SHA256SUMS') -Destination $tempDir
    } else {
        if ($Repo -notmatch '^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$') { throw 'Specify -Repo OWNER/REPO' }
        [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
        $base = "https://github.com/$Repo/releases/download/cli-v$Version"
        Invoke-WebRequest -UseBasicParsing -Uri "$base/$asset" -OutFile (Join-Path $tempDir $asset)
        Invoke-WebRequest -UseBasicParsing -Uri "$base/SHA256SUMS" -OutFile (Join-Path $tempDir 'SHA256SUMS')
    }
    $entries = @(Get-Content -LiteralPath (Join-Path $tempDir 'SHA256SUMS') | Where-Object {
        $_ -cmatch ('^[a-f0-9]{64}  ' + [Regex]::Escape($asset) + '$')
    })
    if ($entries.Count -ne 1) { throw 'Missing or ambiguous checksum' }
    $hasher = [Security.Cryptography.SHA256]::Create()
    $stream = [IO.File]::OpenRead((Join-Path $tempDir $asset))
    try { $actual = [BitConverter]::ToString($hasher.ComputeHash($stream)).Replace('-', '').ToLowerInvariant() }
    finally { $stream.Dispose(); $hasher.Dispose() }
    if ($actual -cne $entries[0].Substring(0,64)) { throw 'SHA-256 mismatch; existing installation unchanged' }
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    $target = Join-Path $InstallDir 'klib.exe'
    if ((Test-Path -LiteralPath $target) -and ((Get-Item -LiteralPath $target).Attributes -band [IO.FileAttributes]::ReparsePoint)) { throw 'Refusing to replace a link' }
    $staged = Join-Path $InstallDir ('.klib-' + [Guid]::NewGuid().ToString('N') + '.exe')
    Copy-Item -LiteralPath (Join-Path $tempDir $asset) -Destination $staged
    & $staged version
    if ($LASTEXITCODE -ne 0) { throw 'Executable self-check failed' }
    if (Test-Path -LiteralPath $target) {
        $backup = Join-Path $InstallDir ('.klib-backup-' + [Guid]::NewGuid().ToString('N') + '.exe')
        [IO.File]::Replace($staged, $target, $backup)
        Remove-Item -LiteralPath $backup -Force
    }
    else { [IO.File]::Move($staged, $target) }
    $staged = $null
    Write-Output "Installed: $target"
    Write-Output "Add this directory to your user PATH: $InstallDir"
} finally {
    if ($staged -and (Test-Path -LiteralPath $staged)) { Remove-Item -LiteralPath $staged -Force }
    Remove-Item -LiteralPath $tempDir -Recurse -Force
}
