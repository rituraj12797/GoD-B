# Launch GOD B UltraKV CLI in a new PowerShell window
$cliPath = Join-Path $PSScriptRoot 'ultra_cli.exe'
if (-Not (Test-Path $cliPath)) {
    Write-Host 'Building ultra_cli.exe...'
    go build -o ultra_cli.exe ultra_cli.go kvstore_ultra.go btree.go
}


Set-Location $PSScriptRoot


$banner = @'
============================
      GOD B - UltraKV CLI    
============================
'@

Start-Process powershell -ArgumentList "-NoExit", "-Command", "Write-Host '$banner'; .\ultra_cli.exe"
