param(
    [Parameter(Mandatory = $true)]
    [string[]]$HostName,

    [Parameter(Mandatory = $true)]
    [string]$KeyPath,

    [string]$User = "root",
    [string]$BinaryPath = "build/lab/linux-amd64/tapx-core",
    [int[]]$Ports = @(46000, 46001)
)

$ErrorActionPreference = "Stop"
$OutputEncoding = [System.Text.UTF8Encoding]::new($false)
[Console]::OutputEncoding = $OutputEncoding

$repo = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$key = (Resolve-Path $KeyPath).Path
$binary = Join-Path $repo $BinaryPath

if (-not (Test-Path -LiteralPath $binary)) {
    throw "missing local lab binary: $binary"
}

$bytes = [IO.File]::ReadAllBytes($binary)
if ($bytes.Length -lt 4 -or $bytes[0] -ne 0x7f -or $bytes[1] -ne 0x45 -or $bytes[2] -ne 0x4c -or $bytes[3] -ne 0x46) {
    throw "local lab binary is not an ELF file: $binary"
}

$sshBase = @("-i", $key, "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new", "-o", "ConnectTimeout=10")
$portList = ($Ports | ForEach-Object { [string]$_ }) -join " "
$hosts = @()
foreach ($item in $HostName) {
    $hosts += ($item -split '[,\s]+' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
}
if ($hosts.Count -eq 0) {
    throw "HostName is empty"
}

foreach ($hostItem in $hosts) {
    Write-Host "===== $hostItem ====="
    $script = @"
set -e
printf 'hostname='; hostname
printf 'kernel='; uname -a
printf 'arch='; uname -m
printf 'tun='; if [ -c /dev/net/tun ]; then echo yes; else echo no; fi
printf 'ip_cmd='; if command -v ip >/dev/null 2>&1; then echo yes; else echo no; fi
printf 'ping_cmd='; if command -v ping >/dev/null 2>&1; then echo yes; else echo no; fi
printf 'ss_cmd='; if command -v ss >/dev/null 2>&1; then echo yes; else echo no; fi
printf 'tmp_leftovers='; find /tmp -maxdepth 1 -type d -name 'tapx-lab-*' 2>/dev/null | wc -l
for p in $portList; do
  if command -v ss >/dev/null 2>&1; then
    if ss -H -lun "( sport = :`$p )" 2>/dev/null | grep -q .; then
      echo "udp_port_`$p=busy"
    else
      echo "udp_port_`$p=free"
    fi
    if ss -H -ltn "( sport = :`$p )" 2>/dev/null | grep -q .; then
      echo "tcp_port_`$p=busy"
    else
      echo "tcp_port_`$p=free"
    fi
  fi
done
"@
    $scriptBytes = [System.Text.UTF8Encoding]::new($false).GetBytes($script)
    $scriptB64 = [Convert]::ToBase64String($scriptBytes)
    & ssh @sshBase "$User@$hostItem" "printf '%s' '$scriptB64' | base64 -d | bash"
    if ($LASTEXITCODE -ne 0) {
        throw "preflight failed on $hostItem"
    }
}
