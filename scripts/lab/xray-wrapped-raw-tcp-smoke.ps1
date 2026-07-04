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
    [string]$XrayPath = "xray",
    [int]$PublicXrayPort = 46443,
    [int]$TapXLocalPort = 46201,
    [int]$ClientXrayPort = 46202,
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

$vlessID = [guid]::NewGuid().ToString()

function New-TapXServerConfig {
    return @"
{
  "Devices": [
    {"ID": "tun-a", "Enabled": true, "Type": "tun", "IfName": "tapxxray0", "MTU": 1400, "IPv4CIDR": "10.79.0.1/30"}
  ],
  "Routes": [
    {"ID": "route-a", "Enabled": true, "DeviceID": "tun-a"}
  ],
  "Listeners": [
    {
      "ID": "tcp-a",
      "Enabled": true,
      "BindHost": "127.0.0.1",
      "BindPort": $TapXLocalPort,
      "Transport": "tcp",
      "RawTCP": {"LengthMode": "uint16", "ReceiveBuffer": 1048576, "SendBuffer": 1048576, "NoDelay": true, "KeepAliveSecond": 30, "ConnectTimeout": 5},
      "Binding": {"RouteID": "route-a"}
    }
  ]
}
"@
}

function New-TapXClientConfig {
    return @"
{
  "Devices": [
    {"ID": "tun-a", "Enabled": true, "Type": "tun", "IfName": "tapxxray0", "MTU": 1400, "IPv4CIDR": "10.79.0.2/30"}
  ],
  "Routes": [
    {"ID": "route-a", "Enabled": true, "DeviceID": "tun-a"}
  ],
  "Connectors": [
    {
      "ID": "tcp-b",
      "Enabled": true,
      "Remote": "127.0.0.1",
      "Port": $ClientXrayPort,
      "Transport": "tcp",
      "RawTCP": {"LengthMode": "uint16", "ReceiveBuffer": 1048576, "SendBuffer": 1048576, "NoDelay": true, "KeepAliveSecond": 30, "ConnectTimeout": 5},
      "Binding": {"RouteID": "route-a"}
    }
  ]
}
"@
}

function New-XrayServerConfig {
    return @"
{
  "log": {"loglevel": "warning"},
  "inbounds": [
    {
      "tag": "tapx-vless-in",
      "listen": "0.0.0.0",
      "port": $PublicXrayPort,
      "protocol": "vless",
      "settings": {
        "clients": [{"id": "$vlessID", "level": 0}],
        "decryption": "none"
      },
      "streamSettings": {
        "network": "tcp",
        "security": "tls",
        "tlsSettings": {
          "certificates": [
            {"certificateFile": "$RemoteDir/server.crt", "keyFile": "$RemoteDir/server.key"}
          ]
        }
      }
    }
  ],
  "outbounds": [
    {
      "tag": "tapx-local",
      "protocol": "freedom",
      "settings": {
        "redirect": "127.0.0.1:$TapXLocalPort",
        "finalRules": [
          {"action": "allow", "network": "tcp", "ip": ["127.0.0.1"], "port": "$TapXLocalPort"}
        ]
      }
    }
  ]
}
"@
}

function New-XrayClientConfig {
    return @"
{
  "log": {"loglevel": "warning"},
  "inbounds": [
    {
      "tag": "tapx-local-in",
      "listen": "127.0.0.1",
      "port": $ClientXrayPort,
      "protocol": "dokodemo-door",
      "settings": {"address": "127.0.0.1", "port": $TapXLocalPort, "network": "tcp"}
    }
  ],
  "outbounds": [
    {
      "tag": "tapx-vless-out",
      "protocol": "vless",
      "settings": {
        "vnext": [
          {
            "address": "$HostA",
            "port": $PublicXrayPort,
            "users": [{"id": "$vlessID", "encryption": "none"}]
          }
        ]
      },
      "streamSettings": {
        "network": "tcp",
        "security": "tls",
        "tlsSettings": {"serverName": "tapx.local", "allowInsecure": true}
      }
    }
  ]
}
"@
}

function Prepare-XrayHost {
    param([Parameter(Mandatory = $true)][string]$HostName)
    Prepare-TapXHost -Context $ctx -HostName $HostName
    Invoke-TapXRemoteScript -Context $ctx -HostName $HostName -Script @"
set -euo pipefail
command -v openssl >/dev/null
if [ -x '$XrayPath' ]; then
  :
else
  command -v '$XrayPath' >/dev/null
fi
"@ | Out-Host
}

function Start-XrayRemote {
    param(
        [Parameter(Mandatory = $true)][string]$HostName,
        [Parameter(Mandatory = $true)][string]$ConfigName,
        [Parameter(Mandatory = $true)][string]$Name
    )
    Invoke-TapXRemoteScript -Context $ctx -HostName $HostName -Script @"
set -euo pipefail
cd '$RemoteDir'
rm -f '$Name.log' '$Name.pid'
nohup '$XrayPath' run -config '$ConfigName' >'$Name.log' 2>&1 &
echo `$! >'$Name.pid'
sleep 1
if ! kill -0 `$(cat '$Name.pid') >/dev/null 2>&1; then
  cat '$Name.log' >&2 || true
  exit 1
fi
"@ | Out-Host
}

function Stop-XrayRemote {
    param([Parameter(Mandatory = $true)][string]$HostName)
    Invoke-TapXRemoteScript -Context $ctx -HostName $HostName -Script @"
set +e
cd '$RemoteDir' 2>/dev/null || exit 0
for pidfile in xray-*.pid; do
  [ -f "`$pidfile" ] || continue
  pid=`$(cat "`$pidfile" 2>/dev/null)
  [ -n "`$pid" ] && kill -TERM "`$pid" >/dev/null 2>&1
done
sleep 1
for pidfile in xray-*.pid; do
  [ -f "`$pidfile" ] || continue
  pid=`$(cat "`$pidfile" 2>/dev/null)
  [ -n "`$pid" ] && kill -KILL "`$pid" >/dev/null 2>&1
done
exit 0
"@ | Out-Host
}

$tapxA = Join-Path $work "tapx-a.json"
$tapxB = Join-Path $work "tapx-b.json"
$xrayA = Join-Path $work "xray-a.json"
$xrayB = Join-Path $work "xray-b.json"
Write-TapXTextFile $tapxA (New-TapXServerConfig)
Write-TapXTextFile $tapxB (New-TapXClientConfig)
Write-TapXTextFile $xrayA (New-XrayServerConfig)
Write-TapXTextFile $xrayB (New-XrayClientConfig)

try {
    Write-Host "prepare remote hosts in $RemoteDir"
    Prepare-XrayHost $HostA
    Prepare-XrayHost $HostB

    Copy-TapXToRemote -Context $ctx -HostName $HostA -LocalPath $tapxA -RemotePath "$RemoteDir/tapx.json"
    Copy-TapXToRemote -Context $ctx -HostName $HostB -LocalPath $tapxB -RemotePath "$RemoteDir/tapx.json"
    Copy-TapXToRemote -Context $ctx -HostName $HostA -LocalPath $xrayA -RemotePath "$RemoteDir/xray.json"
    Copy-TapXToRemote -Context $ctx -HostName $HostB -LocalPath $xrayB -RemotePath "$RemoteDir/xray.json"

    Invoke-TapXRemoteScript -Context $ctx -HostName $HostA -Script @"
set -euo pipefail
cd '$RemoteDir'
openssl req -x509 -newkey rsa:2048 -sha256 -days 1 -nodes -subj '/CN=tapx.local' -keyout server.key -out server.crt >/dev/null 2>&1
chmod 600 server.key server.crt
"@ | Out-Host

    Write-Host "start TapX and Xray wrapped raw TCP/TUN"
    Start-TapXRemoteRuntime -Context $ctx -HostName $HostA -ConfigName "tapx.json" -LogName "tapx.log" -PidName "tapx.pid"
    Start-XrayRemote -HostName $HostA -ConfigName "xray.json" -Name "xray-a"
    Start-XrayRemote -HostName $HostB -ConfigName "xray.json" -Name "xray-b"
    Start-TapXRemoteRuntime -Context $ctx -HostName $HostB -ConfigName "tapx.json" -LogName "tapx.log" -PidName "tapx.pid"

    Invoke-TapXSSH -Context $ctx -HostName $HostA -Command "ping -c 3 -W 2 10.79.0.2" | Out-Host
    Invoke-TapXSSH -Context $ctx -HostName $HostB -Command "ping -c 3 -W 2 10.79.0.1" | Out-Host
    Write-Host "xray-wrapped raw TCP/TUN smoke: ok"
}
finally {
    if ($KeepRemote) {
        Write-Host "keeping remote lab directory: $RemoteDir"
    } else {
        Write-Host "cleanup remote lab directory"
        try { Stop-TapXRemoteRuntime -Context $ctx -HostName $HostA } catch { Write-Warning $_ }
        try { Stop-TapXRemoteRuntime -Context $ctx -HostName $HostB } catch { Write-Warning $_ }
        try { Stop-XrayRemote -HostName $HostA } catch { Write-Warning $_ }
        try { Stop-XrayRemote -HostName $HostB } catch { Write-Warning $_ }
        try { Remove-TapXRemoteDir -Context $ctx -HostName $HostA } catch { Write-Warning $_ }
        try { Remove-TapXRemoteDir -Context $ctx -HostName $HostB } catch { Write-Warning $_ }
    }
    Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
}
