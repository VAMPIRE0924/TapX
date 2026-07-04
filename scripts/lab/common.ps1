$ErrorActionPreference = "Stop"
$OutputEncoding = [System.Text.UTF8Encoding]::new($false)
[Console]::OutputEncoding = $OutputEncoding

function New-TapXLabContext {
    param(
        [Parameter(Mandatory = $true)][string]$Repo,
        [Parameter(Mandatory = $true)][string]$KeyPath,
        [Parameter(Mandatory = $true)][string]$User,
        [Parameter(Mandatory = $true)][string]$BinaryPath,
        [Parameter(Mandatory = $true)][string]$RemoteDir
    )
    $key = (Resolve-Path $KeyPath).Path
    $binary = (Resolve-Path (Join-Path $Repo $BinaryPath)).Path
    Test-TapXELF -Path $binary
    return @{
        Repo = $Repo
        Key = $key
        User = $User
        Binary = $binary
        RemoteDir = $RemoteDir
        SshBase = @("-i", $key, "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new")
    }
}

function Test-TapXELF {
    param([Parameter(Mandatory = $true)][string]$Path)
    if (-not (Test-Path -LiteralPath $Path)) {
        throw "missing local binary: $Path"
    }
    $bytes = [IO.File]::ReadAllBytes($Path)
    if ($bytes.Length -lt 4 -or $bytes[0] -ne 0x7f -or $bytes[1] -ne 0x45 -or $bytes[2] -ne 0x4c -or $bytes[3] -ne 0x46) {
        throw "local binary is not an ELF file: $Path"
    }
}

function Invoke-TapXSSH {
    param(
        [Parameter(Mandatory = $true)][hashtable]$Context,
        [Parameter(Mandatory = $true)][string]$HostName,
        [Parameter(Mandatory = $true)][string]$Command
    )
    $output = & ssh @($Context.SshBase) "$($Context.User)@$HostName" $Command
    if ($LASTEXITCODE -ne 0) {
        throw "ssh command failed on $HostName"
    }
    return $output
}

function Invoke-TapXRemoteScript {
    param(
        [Parameter(Mandatory = $true)][hashtable]$Context,
        [Parameter(Mandatory = $true)][string]$HostName,
        [Parameter(Mandatory = $true)][string]$Script
    )
    $scriptBytes = [System.Text.UTF8Encoding]::new($false).GetBytes($Script)
    $scriptB64 = [Convert]::ToBase64String($scriptBytes)
    $output = & ssh @($Context.SshBase) "$($Context.User)@$HostName" "printf '%s' '$scriptB64' | base64 -d | bash"
    if ($LASTEXITCODE -ne 0) {
        throw "remote script failed on $HostName"
    }
    return $output
}

function Copy-TapXToRemote {
    param(
        [Parameter(Mandatory = $true)][hashtable]$Context,
        [Parameter(Mandatory = $true)][string]$HostName,
        [Parameter(Mandatory = $true)][string]$LocalPath,
        [Parameter(Mandatory = $true)][string]$RemotePath
    )
    & scp @($Context.SshBase) $LocalPath "$($Context.User)@${HostName}:$RemotePath"
    if ($LASTEXITCODE -ne 0) {
        throw "scp failed to $HostName"
    }
}

function Write-TapXTextFile {
    param([string]$Path, [string]$Content)
    [IO.File]::WriteAllText($Path, $Content, [System.Text.UTF8Encoding]::new($false))
}

function Prepare-TapXHost {
    param(
        [Parameter(Mandatory = $true)][hashtable]$Context,
        [Parameter(Mandatory = $true)][string]$HostName,
        [switch]$RequireIperf3
    )
    $iperfCheck = if ($RequireIperf3) { "command -v iperf3 >/dev/null" } else { "true" }
    Invoke-TapXRemoteScript $Context $HostName @"
set -euo pipefail
mkdir -p '$($Context.RemoteDir)'
chmod 700 '$($Context.RemoteDir)'
command -v ip >/dev/null
command -v ping >/dev/null
[ -c /dev/net/tun ]
$iperfCheck
"@ | Out-Host
    Copy-TapXToRemote $Context $HostName $Context.Binary "$($Context.RemoteDir)/tapx-core"
    Invoke-TapXSSH $Context $HostName "chmod +x '$($Context.RemoteDir)/tapx-core'" | Out-Host
}

function Start-TapXRemoteRuntime {
    param(
        [Parameter(Mandatory = $true)][hashtable]$Context,
        [Parameter(Mandatory = $true)][string]$HostName,
        [Parameter(Mandatory = $true)][string]$ConfigName,
        [Parameter(Mandatory = $true)][string]$LogName,
        [Parameter(Mandatory = $true)][string]$PidName
    )
    Invoke-TapXRemoteScript $Context $HostName @"
set -euo pipefail
cd '$($Context.RemoteDir)'
rm -f '$LogName' '$PidName'
nohup ./tapx-core -config '$ConfigName' >'$LogName' 2>&1 &
echo `$! >'$PidName'
for i in `$(seq 1 120); do
  if grep -q 'runtime started' '$LogName'; then
    exit 0
  fi
  if ! kill -0 `$(cat '$PidName') >/dev/null 2>&1; then
    cat '$LogName' >&2 || true
    exit 1
  fi
  sleep 0.1
done
cat '$LogName' >&2 || true
exit 1
"@ | Out-Host
}

function Stop-TapXRemoteRuntime {
    param(
        [Parameter(Mandatory = $true)][hashtable]$Context,
        [Parameter(Mandatory = $true)][string]$HostName
    )
    Invoke-TapXRemoteScript $Context $HostName @"
set +e
cd '$($Context.RemoteDir)' 2>/dev/null || exit 0
for pidfile in *.pid; do
  [ -f "`$pidfile" ] || continue
  pid=`$(cat "`$pidfile" 2>/dev/null)
  [ -n "`$pid" ] && kill -TERM "`$pid" >/dev/null 2>&1
done
sleep 1
for pidfile in *.pid; do
  [ -f "`$pidfile" ] || continue
  pid=`$(cat "`$pidfile" 2>/dev/null)
  [ -n "`$pid" ] && kill -KILL "`$pid" >/dev/null 2>&1
done
exit 0
"@ | Out-Host
}

function Remove-TapXRemoteDir {
    param(
        [Parameter(Mandatory = $true)][hashtable]$Context,
        [Parameter(Mandatory = $true)][string]$HostName
    )
    Invoke-TapXSSH $Context $HostName "rm -rf '$($Context.RemoteDir)'" | Out-Host
}

function New-TapXUdpConfig {
    param(
        [Parameter(Mandatory = $true)][string]$LocalIP,
        [Parameter(Mandatory = $true)][string]$PeerHost,
        [Parameter(Mandatory = $true)][int]$UdpPort
    )
    return @"
{
  "Devices": [
    {"ID": "tun-a", "Enabled": true, "Type": "tun", "IfName": "tapxudp0", "MTU": 1400, "IPv4CIDR": "$LocalIP/30"}
  ],
  "Routes": [
    {"ID": "route-a", "Enabled": true, "DeviceID": "tun-a"}
  ],
  "Listeners": [
    {
      "ID": "udp-a",
      "Enabled": true,
      "BindHost": "0.0.0.0",
      "BindPort": $UdpPort,
      "Transport": "udp",
      "RawUDP": {"PeerMode": "fixed", "FixedPeer": "$($PeerHost):$UdpPort", "ReceiveBuffer": 1048576, "SendBuffer": 1048576, "ReuseAddr": true},
      "Binding": {"RouteID": "route-a"}
    }
  ]
}
"@
}

function New-TapXTapUdpConfig {
    param(
        [Parameter(Mandatory = $true)][string]$LocalIP,
        [Parameter(Mandatory = $true)][string]$PeerHost,
        [Parameter(Mandatory = $true)][int]$UdpPort
    )
    return @"
{
  "Devices": [
    {"ID": "tap-a", "Enabled": true, "Type": "tap", "IfName": "tapxtap0", "MTU": 1400, "IPv4CIDR": "$LocalIP/30"}
  ],
  "Routes": [
    {"ID": "route-a", "Enabled": true, "DeviceID": "tap-a"}
  ],
  "Listeners": [
    {
      "ID": "tap-udp-a",
      "Enabled": true,
      "BindHost": "0.0.0.0",
      "BindPort": $UdpPort,
      "Transport": "udp",
      "RawUDP": {"PeerMode": "fixed", "FixedPeer": "$($PeerHost):$UdpPort", "ReceiveBuffer": 1048576, "SendBuffer": 1048576, "ReuseAddr": true},
      "Binding": {"RouteID": "route-a"}
    }
  ]
}
"@
}

function New-TapXTcpListenerConfig {
    param([Parameter(Mandatory = $true)][int]$TcpPort)
    return @"
{
  "Devices": [
    {"ID": "tun-a", "Enabled": true, "Type": "tun", "IfName": "tapxtcp0", "MTU": 1400, "IPv4CIDR": "10.78.0.1/30"}
  ],
  "Routes": [
    {"ID": "route-a", "Enabled": true, "DeviceID": "tun-a"}
  ],
  "Listeners": [
    {
      "ID": "tcp-a",
      "Enabled": true,
      "BindHost": "0.0.0.0",
      "BindPort": $TcpPort,
      "Transport": "tcp",
      "RawTCP": {"LengthMode": "uint16", "ReceiveBuffer": 1048576, "SendBuffer": 1048576, "NoDelay": true, "KeepAliveSecond": 30, "ConnectTimeout": 5},
      "Binding": {"RouteID": "route-a"}
    }
  ]
}
"@
}

function New-TapXTcpConnectorConfig {
    param(
        [Parameter(Mandatory = $true)][string]$HostA,
        [Parameter(Mandatory = $true)][int]$TcpPort
    )
    return @"
{
  "Devices": [
    {"ID": "tun-a", "Enabled": true, "Type": "tun", "IfName": "tapxtcp0", "MTU": 1400, "IPv4CIDR": "10.78.0.2/30"}
  ],
  "Routes": [
    {"ID": "route-a", "Enabled": true, "DeviceID": "tun-a"}
  ],
  "Connectors": [
    {
      "ID": "tcp-b",
      "Enabled": true,
      "Remote": "$HostA",
      "Port": $TcpPort,
      "Transport": "tcp",
      "RawTCP": {"LengthMode": "uint16", "ReceiveBuffer": 1048576, "SendBuffer": 1048576, "NoDelay": true, "KeepAliveSecond": 30, "ConnectTimeout": 5},
      "Binding": {"RouteID": "route-a"}
    }
  ]
}
"@
}
