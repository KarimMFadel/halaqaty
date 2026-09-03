param([Parameter(Mandatory = $true)][string]$Profile)

$ErrorActionPreference = 'Stop'
$blocks = @{}
try {
    foreach ($line in Get-Content -LiteralPath $Profile) {
        if ($line -match '^mode:') { continue }
        $fields = $line -split '\s+'
        if ($fields.Count -lt 3) { continue }
        $key = $fields[0]
        if (-not $blocks.ContainsKey($key)) {
            $blocks[$key] = @{
                Statements = [int]$fields[1]
                Covered = $false
            }
        }
        if ([int]$fields[2] -gt 0) { $blocks[$key].Covered = $true }
    }
    $total = 0
    $covered = 0
    foreach ($block in $blocks.Values) {
        $total += [int]$block['Statements']
        if ([bool]$block['Covered']) { $covered += [int]$block['Statements'] }
    }
    if ($total -eq 0) { throw 'coverage profile contains no statements' }
    $coverage = [math]::Round(100 * $covered / $total, 2)
    Write-Host "Feature 003 aggregate backend/internal coverage: $coverage% (required >= 80%)"
    if ($coverage -lt 80) { exit 1 }
} finally {
    Remove-Item -LiteralPath $Profile -Force -ErrorAction SilentlyContinue
}
