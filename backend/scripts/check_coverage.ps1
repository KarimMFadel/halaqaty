param([Parameter(Mandatory = $true)][string]$Profile)

# Canonical project-wide coverage floor checker (constitution section VI: >=80%
# aggregate over backend/internal/). Consumes a Go coverage profile produced by
# `make coverage` (backend/Makefile), which runs the unit, contract, and
# integration suites together with -coverpkg=./internal/... .
#
# NOTE: a unit-only profile (`go test -short ./...`) undercounts coverage,
# because tests/contract and tests/integration are behind build tags. Always
# measure through `make coverage`.

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
    Write-Host "Aggregate backend/internal coverage: $coverage% (required >= 80%)"
    if ($coverage -lt 80) { exit 1 }
} finally {
    Remove-Item -LiteralPath $Profile -Force -ErrorAction SilentlyContinue
}
