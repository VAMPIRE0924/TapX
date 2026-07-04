param(
    [Parameter(Mandatory = $true)]
    [string[]]$HostName,

    [Parameter(Mandatory = $true)]
    [string]$KeyPath,

    [string]$User = "root"
)

$ErrorActionPreference = "Stop"

foreach ($hostItem in $HostName) {
    Write-Host "===== $hostItem ====="
    ssh -i $KeyPath -o BatchMode=yes -o StrictHostKeyChecking=accept-new "$User@$hostItem" @'
set -e
printf 'hostname='; hostname
printf 'kernel='; uname -a
printf 'os='; . /etc/os-release 2>/dev/null && printf "%s %s\n" "$NAME" "$VERSION_ID" || true
printf 'arch='; uname -m
printf 'cpu='; nproc
printf 'mem='; awk '/MemTotal/ {print $2 " kB"}' /proc/meminfo
printf 'disk='; df -h / | awk 'NR==2 {print $2 " total, " $4 " free"}'
printf 'tun='; if [ -c /dev/net/tun ]; then echo yes; else echo no; fi
'@
}
