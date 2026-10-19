# Deletes the data of the two-mode stand (Windows): stops it and removes the
# volumes of a release (database, S3, Caddy state, Redpanda, course-graph).
# The models course-graph downloaded and the certificates of the start page
# stay unless asked. The main stand (ai-tutor_*) is not touched. Linux and
# macOS use linux-dual-reset.sh. See dual/README.md.

# Native tools report progress on stderr; failures are checked by exit code.
$ErrorActionPreference = 'Continue'
Set-StrictMode -Version 2

function Show-Usage {
    [Console]::Error.WriteLine(@'
Usage: .\deploy\windows-dual-reset.ps1 [learn|exam|all] [--yes]

  learn|exam|all  whose data to delete [all]
  --yes           do not ask for confirmation

Environment, set with $env:NAME = 'value':
  DUAL_RESET_MODELS=true  also delete the models course-graph downloaded
  DUAL_RESET_TLS=true     also delete the certificates of the start page
The stand stays stopped; start it again with .\deploy\windows-dual.ps1 up.
'@)
    exit 2
}

$target = 'all'
$yes = $false
foreach ($arg in $args) {
    switch ($arg) {
        { $_ -in 'learn', 'exam', 'all' } { $target = $arg; continue }
        { $_ -in '--yes', '-y' } { $yes = $true; continue }
        default { Show-Usage }
    }
}

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    [Console]::Error.WriteLine('Docker is required.')
    exit 1
}

if ($target -eq 'all') { $projects = @('tutor-learn', 'tutor-exam') } else { $projects = @("tutor-$target") }
if ($env:DUAL_RESET_TLS -eq 'true') { $projects += 'tutor-dual' }

# The volumes to delete: every volume of the chosen projects, but the models
# unless asked.
$volumes = @()
foreach ($project in $projects) {
    foreach ($volume in @(docker volume ls -q)) {
        if ($volume -notlike "${project}_*") { continue }
        if ($volume -like '*_course_graph_models' -and $env:DUAL_RESET_MODELS -ne 'true') { continue }
        $volumes += $volume
    }
}

if ($volumes.Count -eq 0) {
    Write-Output "No data of $target to delete."
    exit 0
}

Write-Output 'These volumes will be deleted for good:'
$volumes | ForEach-Object { Write-Output "  $_" }
if (-not $yes) {
    $answer = Read-Host 'Type yes to delete'
    if ($answer -ne 'yes') {
        Write-Output 'Nothing deleted.'
        exit 1
    }
}

# Containers hold their volumes; stop the whole stand first.
& (Join-Path $PSScriptRoot 'windows-dual.ps1') down
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
docker volume rm @volumes
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Output 'Deleted. Start the stand again with: .\deploy\windows-dual.ps1 up'
