param(
    [Parameter(Mandatory = $true)]
    [string]$HostA,

    [Parameter(Mandatory = $true)]
    [string]$HostB,

    [Parameter(Mandatory = $true)]
    [string]$KeyPath,

    [string]$User = "root",
    [string]$BinaryPath = "build/lab/linux-amd64/tapx-core",
    [string]$RemoteDir = "",
    [int]$TcpTLSPort = 46601,
    [int]$UdpDTLSPort = 46602,
    [ValidateSet("tls", "dtls", "both")]
    [string]$Mode = "both",
    [switch]$Build,
    [switch]$KeepRemote
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "common.ps1")

$repo = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$runId = [DateTimeOffset]::UtcNow.ToString("yyyyMMddHHmmss")
if ([string]::IsNullOrWhiteSpace($RemoteDir)) {
    $RemoteDir = "/tmp/tapx-lab-$runId"
}
$work = Join-Path ([IO.Path]::GetTempPath()) "tapx-lab-$runId"
New-Item -ItemType Directory -Force -Path $work | Out-Null

if ($Build) {
    & (Join-Path $PSScriptRoot "build-linux-amd64.ps1") -OutputPath $BinaryPath
}

$ctx = New-TapXLabContext `
    -Repo $repo `
    -KeyPath $KeyPath `
    -User $User `
    -BinaryPath $BinaryPath `
    -RemoteDir $RemoteDir

$certLocal = Join-Path $work "server.crt"
$tlsA = Join-Path $work "tls-a.json"
$tlsB = Join-Path $work "tls-b.json"
$dtlsA = Join-Path $work "dtls-a.json"
$dtlsB = Join-Path $work "dtls-b.json"
$vkeyTLS = "tapx-remote-tls-$runId"
$vkeyDTLS = "tapx-remote-dtls-$runId"
$serverName = "tapx.remote"

function New-TapXTLSListenerConfig {
    return @"
{
  "Devices": [
    {"ID": "tun-a", "Enabled": true, "Type": "tun", "IfName": "tapxtls0", "MTU": 1400, "IPv4CIDR": "10.94.0.1/30"}
  ],
  "VKeys": [
    {"ID": "vk-a", "Enabled": true, "Value": "$vkeyTLS"}
  ],
  "Routes": [
    {"ID": "route-a", "Enabled": true, "DeviceID": "tun-a", "VKeyID": "vk-a"}
  ],
  "Listeners": [
    {
      "ID": "tcp-tls-a",
      "Enabled": true,
      "BindHost": "0.0.0.0",
      "BindPort": $TcpTLSPort,
      "Transport": "tcp",
      "RawTCP": {
        "LengthMode": "uint16",
        "ReceiveBuffer": 1048576,
        "SendBuffer": 1048576,
        "NoDelay": true,
        "KeepAliveSecond": 30,
        "ConnectTimeout": 5,
        "TLS": {
          "Enabled": true,
          "CertFile": "$RemoteDir/server.crt",
          "KeyFile": "$RemoteDir/server.key",
          "ALPN": ["tapx"],
          "MinVersion": "1.2"
        }
      },
      "Binding": {"RouteID": "route-a"}
    }
  ]
}
"@
}

function New-TapXTLSConnectorConfig {
    return @"
{
  "Devices": [
    {"ID": "tun-b", "Enabled": true, "Type": "tun", "IfName": "tapxtls0", "MTU": 1400, "IPv4CIDR": "10.94.0.2/30"}
  ],
  "VKeys": [
    {"ID": "vk-b", "Enabled": true, "Value": "$vkeyTLS"}
  ],
  "Routes": [
    {"ID": "route-b", "Enabled": true, "DeviceID": "tun-b", "VKeyID": "vk-b"}
  ],
  "Connectors": [
    {
      "ID": "tcp-tls-b",
      "Enabled": true,
      "Remote": "$HostA",
      "Port": $TcpTLSPort,
      "Transport": "tcp",
      "RawTCP": {
        "LengthMode": "uint16",
        "ReceiveBuffer": 1048576,
        "SendBuffer": 1048576,
        "NoDelay": true,
        "KeepAliveSecond": 30,
        "ConnectTimeout": 5,
        "TLS": {
          "Enabled": true,
          "CAFile": "$RemoteDir/server.crt",
          "ServerName": "$serverName",
          "ALPN": ["tapx"],
          "MinVersion": "1.2"
        }
      },
      "Binding": {"RouteID": "route-b"}
    }
  ]
}
"@
}

function New-TapXDTLSListenerConfig {
    return @"
{
  "Devices": [
    {"ID": "tun-a", "Enabled": true, "Type": "tun", "IfName": "tapxdtls0", "MTU": 1400, "IPv4CIDR": "10.95.0.1/30"}
  ],
  "VKeys": [
    {"ID": "vk-a", "Enabled": true, "Value": "$vkeyDTLS"}
  ],
  "Routes": [
    {"ID": "route-a", "Enabled": true, "DeviceID": "tun-a", "VKeyID": "vk-a"}
  ],
  "Listeners": [
    {
      "ID": "udp-dtls-a",
      "Enabled": true,
      "BindHost": "0.0.0.0",
      "BindPort": $UdpDTLSPort,
      "Transport": "udp",
      "RawUDP": {
        "PeerMode": "learn",
        "ReceiveBuffer": 1048576,
        "SendBuffer": 1048576,
        "ReuseAddr": true,
        "DTLS": {
          "Enabled": true,
          "CertFile": "$RemoteDir/server.crt",
          "KeyFile": "$RemoteDir/server.key",
          "ALPN": ["tapx"],
          "MTU": 1200,
          "ReplayWindow": 64
        }
      },
      "Binding": {"RouteID": "route-a"}
    }
  ]
}
"@
}

function New-TapXDTLSConnectorConfig {
    return @"
{
  "Devices": [
    {"ID": "tun-b", "Enabled": true, "Type": "tun", "IfName": "tapxdtls0", "MTU": 1400, "IPv4CIDR": "10.95.0.2/30"}
  ],
  "VKeys": [
    {"ID": "vk-b", "Enabled": true, "Value": "$vkeyDTLS"}
  ],
  "Routes": [
    {"ID": "route-b", "Enabled": true, "DeviceID": "tun-b", "VKeyID": "vk-b"}
  ],
  "Connectors": [
    {
      "ID": "udp-dtls-b",
      "Enabled": true,
      "Remote": "$HostA",
      "Port": $UdpDTLSPort,
      "Transport": "udp",
      "RawUDP": {
        "PeerMode": "fixed",
        "FixedPeer": "$($HostA):$UdpDTLSPort",
        "ReceiveBuffer": 1048576,
        "SendBuffer": 1048576,
        "ReuseAddr": true,
        "DTLS": {
          "Enabled": true,
          "CAFile": "$RemoteDir/server.crt",
          "ServerName": "$serverName",
          "ALPN": ["tapx"],
          "MTU": 1200,
          "ReplayWindow": 64
        }
      },
      "Binding": {"RouteID": "route-b"}
    }
  ]
}
"@
}

function New-TapXRemoteCertificate {
    Invoke-TapXRemoteScript $ctx $HostA @"
set -euo pipefail
cd '$RemoteDir'
command -v openssl >/dev/null
openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout server.key \
  -out server.crt \
  -days 1 \
  -subj '/CN=$serverName' \
  -addext 'subjectAltName = DNS:$serverName,IP:$HostA' >/dev/null 2>&1
"@ | Out-Host
    & scp @($ctx.SshBase) "$($ctx.User)@${HostA}:$RemoteDir/server.crt" $certLocal
    if ($LASTEXITCODE -ne 0) {
        throw "scp certificate from $HostA failed"
    }
    Copy-TapXToRemote -Context $ctx -HostName $HostB -LocalPath $certLocal -RemotePath "$RemoteDir/server.crt"
}

function Invoke-TapXProtectedPair {
    param(
        [Parameter(Mandatory = $true)][string]$Name,
        [Parameter(Mandatory = $true)][string]$IfName,
        [Parameter(Mandatory = $true)][string]$ConfigA,
        [Parameter(Mandatory = $true)][string]$ConfigB,
        [Parameter(Mandatory = $true)][string]$PingA,
        [Parameter(Mandatory = $true)][string]$PingB
    )
    Copy-TapXToRemote -Context $ctx -HostName $HostA -LocalPath $ConfigA -RemotePath "$RemoteDir/$Name-a.json"
    Copy-TapXToRemote -Context $ctx -HostName $HostB -LocalPath $ConfigB -RemotePath "$RemoteDir/$Name-b.json"
    Start-TapXRemoteRuntime -Context $ctx -HostName $HostA -ConfigName "$Name-a.json" -LogName "$Name-a.log" -PidName "$Name-a.pid"
    Start-TapXRemoteRuntime -Context $ctx -HostName $HostB -ConfigName "$Name-b.json" -LogName "$Name-b.log" -PidName "$Name-b.pid"
    Write-Host "HostA $Name interface"
    Invoke-TapXSSH -Context $ctx -HostName $HostA -Command "ip a show dev $IfName; ip -d addr show dev $IfName" | Out-Host
    Write-Host "HostB $Name interface"
    Invoke-TapXSSH -Context $ctx -HostName $HostB -Command "ip a show dev $IfName; ip -d addr show dev $IfName" | Out-Host
    Invoke-TapXSSH -Context $ctx -HostName $HostA -Command "ping -c 3 -W 2 $PingA" | Out-Host
    Invoke-TapXSSH -Context $ctx -HostName $HostB -Command "ping -c 3 -W 2 $PingB" | Out-Host
    Stop-TapXRemoteRuntime -Context $ctx -HostName $HostA
    Stop-TapXRemoteRuntime -Context $ctx -HostName $HostB
    Write-Host "$Name smoke: ok"
}

Write-TapXTextFile $tlsA (New-TapXTLSListenerConfig)
Write-TapXTextFile $tlsB (New-TapXTLSConnectorConfig)
Write-TapXTextFile $dtlsA (New-TapXDTLSListenerConfig)
Write-TapXTextFile $dtlsB (New-TapXDTLSConnectorConfig)

try {
    Write-Host "prepare remote hosts in $RemoteDir"
    Prepare-TapXHost -Context $ctx -HostName $HostA
    Prepare-TapXHost -Context $ctx -HostName $HostB
    New-TapXRemoteCertificate

    if ($Mode -eq "tls" -or $Mode -eq "both") {
        Write-Host "run Raw TCP/TLS/TUN smoke"
        Invoke-TapXProtectedPair -Name "tls" -IfName "tapxtls0" -ConfigA $tlsA -ConfigB $tlsB -PingA "10.94.0.2" -PingB "10.94.0.1"
    }

    if ($Mode -eq "dtls" -or $Mode -eq "both") {
        Write-Host "run Raw UDP/DTLS/TUN smoke"
        Invoke-TapXProtectedPair -Name "dtls" -IfName "tapxdtls0" -ConfigA $dtlsA -ConfigB $dtlsB -PingA "10.95.0.2" -PingB "10.95.0.1"
    }
}
finally {
    if ($KeepRemote) {
        Write-Host "keeping remote lab directory: $RemoteDir"
    } else {
        Write-Host "cleanup remote lab directory"
        try { Stop-TapXRemoteRuntime -Context $ctx -HostName $HostA } catch { Write-Warning $_ }
        try { Stop-TapXRemoteRuntime -Context $ctx -HostName $HostB } catch { Write-Warning $_ }
        try { Remove-TapXRemoteDir -Context $ctx -HostName $HostA } catch { Write-Warning $_ }
        try { Remove-TapXRemoteDir -Context $ctx -HostName $HostB } catch { Write-Warning $_ }
    }
    Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
}
