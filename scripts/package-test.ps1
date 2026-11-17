param(
    [string]$BuildName = 'campus-lost-found-windows-x64',
    [string]$ExecutableName = 'CampusLostFound.exe'
)

$ErrorActionPreference = 'Stop'
if ($BuildName -notmatch '^[a-z0-9-]+$') { throw 'Use a simple lowercase build name.' }
if ($ExecutableName -notmatch '^CampusLostFound(?:-[a-zA-Z0-9-]+)?\.exe$') { throw 'Choose a CampusLostFound executable from desktop/build/bin.' }
$projectRoot = Split-Path $PSScriptRoot -Parent
$executable = Join-Path $projectRoot "desktop/build/bin/$ExecutableName"
if (-not (Test-Path -LiteralPath $executable)) { throw 'Build the Windows executable first.' }
$releaseDirectory = Join-Path $projectRoot 'release'
$packageDirectory = Join-Path $releaseDirectory $BuildName
$null = New-Item -ItemType Directory -Force -Path $packageDirectory
$packageFiles = @('CampusLostFound.exe', 'TEST-GUIDE.md', 'FEEDBACK.md', 'test-map.md', 'app-logo.png', 'SHA256SUMS.txt')
Copy-Item -LiteralPath $executable -Destination (Join-Path $packageDirectory 'CampusLostFound.exe')
foreach ($document in @('TEST-GUIDE.md', 'FEEDBACK.md', 'test-map.md')) {
    Copy-Item -LiteralPath (Join-Path $projectRoot "docs/$document") -Destination $packageDirectory
}
Copy-Item -LiteralPath (Join-Path $projectRoot 'src/assets/app-logo.png') -Destination $packageDirectory
$executableHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $executable).Hash.ToLowerInvariant()
Set-Content -LiteralPath (Join-Path $packageDirectory 'SHA256SUMS.txt') -Value "$executableHash  CampusLostFound.exe" -Encoding ascii
$archivePath = Join-Path $releaseDirectory "$BuildName.zip"
Compress-Archive -LiteralPath ($packageFiles | ForEach-Object { Join-Path $packageDirectory $_ }) -DestinationPath $archivePath -Force
$archiveHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $archivePath).Hash.ToLowerInvariant()
Set-Content -LiteralPath "$archivePath.sha256" -Value "$archiveHash  $BuildName.zip" -Encoding ascii
Write-Output $archivePath
Write-Output "SHA256: $archiveHash"
