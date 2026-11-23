param([Parameter(Mandatory)][string]$SourceFile)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $root
$lineNumber = 0
foreach ($line in Get-Content -LiteralPath (Join-Path $root '.env') -Encoding UTF8) {
    $lineNumber++
    if ($line -match '^\s*(#|$)') { continue }
    if ($line -notmatch '^([A-Za-z_][A-Za-z0-9_]*)=(.*)$') {
        throw "Invalid .env entry at line $lineNumber (expected KEY=value)"
    }
    [Environment]::SetEnvironmentVariable($Matches[1], $Matches[2], 'Process')
}

$package = Resolve-Path -LiteralPath (Split-Path -Parent $SourceFile) -Relative
go run $package
exit $LASTEXITCODE
