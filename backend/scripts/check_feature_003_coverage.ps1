param([Parameter(Mandatory = $true)][string]$Profile)

$total = 0
$covered = 0
try {
    foreach ($line in Get-Content -LiteralPath $Profile) {
        if ($line -match '^mode:') { continue }
        $fields = $line -split '\s+'
        if ($fields.Count -lt 3) { continue }
        $total += [int]$fields[1]
        if ([int]$fields[2] -gt 0) { $covered += [int]$fields[1] }
    }
    if ($total -eq 0) { throw 'coverage profile contains no statements' }
    $coverage = [math]::Round(100 * $covered / $total, 2)
    Write-Host "Feature 003 aggregate backend/internal coverage: $coverage% (required >= 80%)"
    if ($coverage -lt 80) { exit 1 }
} finally {
    Remove-Item -LiteralPath $Profile -Force -ErrorAction SilentlyContinue
}
