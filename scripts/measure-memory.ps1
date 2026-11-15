param(
    [Parameter(Mandatory = $true)][int]$AppProcessId,
    [string]$OutputPath = 'tmp/memory-sample.json'
)

$ErrorActionPreference = 'Stop'
$processInventory = @(Get-CimInstance Win32_Process)
$appRoot = $processInventory | Where-Object ProcessId -EQ $AppProcessId
if (-not $appRoot -or $appRoot.Name -ne 'CampusLostFound.exe') {
    throw 'Select the running CampusLostFound.exe process.'
}
$appIds = [System.Collections.Generic.HashSet[int]]::new()
$null = $appIds.Add($AppProcessId)
do {
    $added = $false
    foreach ($entry in $processInventory) {
        if ($appIds.Contains([int]$entry.ParentProcessId) -and $appIds.Add([int]$entry.ProcessId)) { $added = $true }
    }
} while ($added)

$counters = Get-CimInstance Win32_PerfFormattedData_PerfProc_Process
$samples = foreach ($entry in $processInventory) {
    if (-not $appIds.Contains([int]$entry.ProcessId)) { continue }
    $process = Get-Process -Id $entry.ProcessId -ErrorAction SilentlyContinue
    if (-not $process) { continue }
    $counter = $counters | Where-Object IDProcess -EQ $entry.ProcessId | Select-Object -First 1
    [PSCustomObject]@{
        processId = $entry.ProcessId
        name = $entry.Name
        workingSetMiB = [math]::Round($process.WorkingSet64 / 1MB, 2)
        privateWorkingSetMiB = if ($counter) { [math]::Round($counter.WorkingSetPrivate / 1MB, 2) } else { $null }
        privateCommittedMiB = [math]::Round($process.PrivateMemorySize64 / 1MB, 2)
    }
}
$result = [PSCustomObject]@{
    timestampUtc = [datetime]::UtcNow.ToString('o')
    rootProcessId = $AppProcessId
    processCount = @($samples).Count
    totalWorkingSetMiB = [math]::Round(($samples | Measure-Object workingSetMiB -Sum).Sum, 2)
    totalPrivateWorkingSetMiB = [math]::Round(($samples | Measure-Object privateWorkingSetMiB -Sum).Sum, 2)
    totalPrivateCommittedMiB = [math]::Round(($samples | Measure-Object privateCommittedMiB -Sum).Sum, 2)
    note = 'Working set sums may count shared pages more than once. Private working set is resident private RAM. Private committed memory is not the same as resident RAM.'
    processes = @($samples)
}
$result | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $OutputPath
$result | Select-Object processCount, totalWorkingSetMiB, totalPrivateWorkingSetMiB, totalPrivateCommittedMiB
