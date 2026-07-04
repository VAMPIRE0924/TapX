param(
    [Parameter(Mandatory = $true)]
    [string[]]$HostName,

    [Parameter(Mandatory = $true)]
    [string]$LoginKeyPath,

    [Parameter(Mandatory = $true)]
    [string]$PublicKeyPath
    ,
    [string]$User = "root"
)

$ErrorActionPreference = "Stop"
$pub = (Get-Content $PublicKeyPath -Raw).Trim()
$encoded = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($pub))

foreach ($hostItem in $HostName) {
    Write-Host "===== removing key from $hostItem ====="
    ssh -i $LoginKeyPath -o BatchMode=yes "$User@$hostItem" "python3 - <<'PY'
import base64
from pathlib import Path
target = base64.b64decode('$encoded').decode()
path = Path('/root/.ssh/authorized_keys')
if path.exists():
    lines = [line.rstrip('\n') for line in path.read_text().splitlines()]
    lines = [line for line in lines if line.strip() != target.strip()]
    path.write_text('\n'.join(lines) + ('\n' if lines else ''))
print('ok')
PY"
}
