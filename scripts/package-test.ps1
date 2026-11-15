param([string]$BuildName = 'campus-lost-found-iteration-1-windows-x64')

$ErrorActionPreference = 'Stop'
if ($BuildName -notmatch '^[a-z0-9-]+$') { throw 'Use a simple lowercase build name.' }
$projectRoot = Split-Path $PSScriptRoot -Parent
$executable = Join-Path $projectRoot 'desktop/build/bin/CampusLostFound.exe'
if (-not (Test-Path -LiteralPath $executable)) { throw 'Build the Windows executable first.' }
$releaseDirectory = Join-Path $projectRoot 'release'
$packageDirectory = Join-Path $releaseDirectory $BuildName
$null = New-Item -ItemType Directory -Force -Path $packageDirectory
Copy-Item -LiteralPath $executable -Destination $packageDirectory
foreach ($document in @('TEST-GUIDE.md', 'FEEDBACK.md', 'test-map.md')) {
    Copy-Item -LiteralPath (Join-Path $projectRoot "docs/$document") -Destination $packageDirectory
}
Copy-Item -LiteralPath (Join-Path $projectRoot 'src/assets/app-logo.png') -Destination $packageDirectory
$executableHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $executable).Hash.ToLowerInvariant()
Set-Content -LiteralPath (Join-Path $packageDirectory 'SHA256SUMS.txt') -Value "$executableHash  CampusLostFound.exe" -Encoding ascii
$archivePath = Join-Path $releaseDirectory "$BuildName.zip"
Compress-Archive -LiteralPath $packageDirectory -DestinationPath $archivePath -Force
$archiveHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $archivePath).Hash.ToLowerInvariant()
Set-Content -LiteralPath "$archivePath.sha256" -Value "$archiveHash  $BuildName.zip" -Encoding ascii
Write-Output $archivePath
Write-Output "SHA256: $archiveHash"
