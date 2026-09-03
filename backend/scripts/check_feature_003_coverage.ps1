param([Parameter(Mandatory = $true)][string]$Profile)

$blocks = @{}
try {
    foreach ($line in Get-Content -LiteralPath $Profile) {
        if ($line -match '^mode:') { continue }
        $fields = $line -split '\s+'
        if ($fields.Count -lt 3) { continue }
        $key = "$($fields[0])|$($fields[1])"
        if (-not $blocks.ContainsKey($key)) { $blocks[$key] = 0 }
        if ([int]$fields[2] -gt 0) { $blocks[$key] = 1 }
    }
	$total = $blocks.Count
	$covered = @($blocks.Values | Where-Object { $_ -gt 0 }).Count
    if ($total -eq 0) { throw 'coverage profile contains no statements' }
    $coverage = [math]::Round(100 * $covered / $total, 2)
    Write-Host "Feature 003 aggregate backend/internal coverage: $coverage% (required >= 80%)"
    if ($coverage -lt 80) { exit 1 }
} finally {
    Remove-Item -LiteralPath $Profile -Force -ErrorAction SilentlyContinue
}
