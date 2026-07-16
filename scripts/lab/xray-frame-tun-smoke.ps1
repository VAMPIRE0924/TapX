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
    [int]$XrayPort = 46210,
    [ValidateSet("tun", "tap", "both")]
    [string]$Mode = "tun",
    [ValidateSet("embedded", "external", "both")]
    [string]$Runtime = "embedded",
    [string]$XrayBinaryPath = "build/lab/xray-linux-amd64",
    [string]$XrayDownloadURL = "",
    [switch]$Throughput,
    [ValidateRange(1, 30)]
    [int]$ThroughputSeconds = 5,
    [switch]$Build,
    [switch]$UseInstalledBinary,
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

function Invoke-TunnelThroughputDirection {
    param(
        [Parameter(Mandatory = $true)][string]$ServerHost,
        [Parameter(Mandatory = $true)][string]$ClientHost,
        [Parameter(Mandatory = $true)][string]$ServerIP,
        [Parameter(Mandatory = $true)][int]$Port,
        [Parameter(Mandatory = $true)][string]$Label
    )

    Invoke-TapXRemoteScript -Context $ctx -HostName $ServerHost -Script @"
set -euo pipefail
rm -f '$RemoteDir/throughput-$Port.json' '$RemoteDir/throughput-$Port.pid'
nohup python3 -u -c 'import json,socket,time; s=socket.socket(); s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1); s.bind(("$ServerIP",$Port)); s.listen(1); c,_=s.accept(); start=time.monotonic(); total=0
while True:
 d=c.recv(1048576)
 if not d: break
 total+=len(d)
elapsed=max(time.monotonic()-start,1e-9); print(json.dumps({"bytes":total,"seconds":elapsed,"bps":int(total*8/elapsed)}),flush=True); c.close(); s.close()' >'$RemoteDir/throughput-$Port.json' 2>&1 &
echo `$! >'$RemoteDir/throughput-$Port.pid'
for i in `$(seq 1 50); do ss -ltn | grep -q '${ServerIP}:$Port' && exit 0; sleep 0.1; done
cat '$RemoteDir/throughput-$Port.json' >&2 || true
exit 1
"@ | Out-Null

    $clientJSON = Invoke-TapXRemoteScript -Context $ctx -HostName $ClientHost -Script @"
set -euo pipefail
python3 - <<'PY'
import json
import socket
import time

s = socket.socket()
s.connect(("$ServerIP", $Port))
payload = bytes(262144)
deadline = time.monotonic() + $ThroughputSeconds
started = time.monotonic()
total = 0
while time.monotonic() < deadline:
    s.sendall(payload)
    total += len(payload)
s.shutdown(socket.SHUT_WR)
elapsed = max(time.monotonic() - started, 1e-9)
print(json.dumps({"bytes": total, "seconds": elapsed, "bps": int(total * 8 / elapsed)}))
s.close()
PY
"@
    $serverJSON = Invoke-TapXRemoteScript -Context $ctx -HostName $ServerHost -Script @"
set -euo pipefail
pid=`$(cat '$RemoteDir/throughput-$Port.pid')
for i in `$(seq 1 100); do kill -0 `$pid >/dev/null 2>&1 || break; sleep 0.1; done
wait `$pid 2>/dev/null || true
cat '$RemoteDir/throughput-$Port.json'
"@
    $client = $clientJSON | ConvertFrom-Json
    $server = $serverJSON | ConvertFrom-Json
    if ([uint64]$server.bytes -eq 0 -or [uint64]$server.bytes -ne [uint64]$client.bytes) {
        throw "$Label throughput byte mismatch: client=$($client.bytes) server=$($server.bytes)"
    }
    Write-Host ("{0}: {1:N2} Mbit/s, {2:N2} MiB" -f $Label, ([double]$server.bps / 1000000), ([double]$server.bytes / 1MB))
}

function New-TapXXrayFrameServerConfig {
    param(
        [Parameter(Mandatory = $true)][string]$DeviceType,
        [Parameter(Mandatory = $true)][string]$IfName,
        [Parameter(Mandatory = $true)][string]$LocalCIDR,
        [Parameter(Mandatory = $true)][string]$XrayRuntime
    )
    return @"
{
  "Devices": [
    {"ID": "dev-xray", "Enabled": true, "Type": "$DeviceType", "IfName": "$IfName", "MTU": 1500, "LinkAutoOptimize": true, "IPv4CIDR": "$LocalCIDR"}
  ],
  "XrayProfiles": [
    {
      "ID": "xray-server",
      "Enabled": true,
      "Runtime": "$XrayRuntime",
      "InboundProtocol": "vless",
      "InboundSettingsJSON": "{\"clients\":[{\"id\":\"$vlessID\",\"level\":0}],\"decryption\":\"none\"}",
      "Network": "tcp",
      "Security": "none",
      "StreamSettingsJSON": "{}",
      "AdvancedJSON": "{\"outbounds\":[{\"tag\":\"direct\",\"protocol\":\"freedom\"}]}"
    }
  ],
  "Listeners": [
    {
      "ID": "xray-frame-in",
      "Enabled": true,
      "BindHost": "0.0.0.0",
      "BindPort": $XrayPort,
      "Transport": "xray",
      "XrayProfileID": "xray-server",
      "RawTCP": {"LengthMode": "uint16"},
      "Binding": {"DeviceID": "dev-xray"}
    }
  ],
  "Settings": [
    {"ID":"global","Enabled":true,"ExternalXrayPath":"$RemoteDir/xray","DataDir":"$RemoteDir/data","LogLevel":"warn"}
  ]
}
"@
}

function New-TapXXrayFrameClientConfig {
    param(
        [Parameter(Mandatory = $true)][string]$DeviceType,
        [Parameter(Mandatory = $true)][string]$IfName,
        [Parameter(Mandatory = $true)][string]$LocalCIDR,
        [Parameter(Mandatory = $true)][string]$XrayRuntime
    )
    return @"
{
  "Devices": [
    {"ID": "dev-xray", "Enabled": true, "Type": "$DeviceType", "IfName": "$IfName", "MTU": 1500, "LinkAutoOptimize": true, "IPv4CIDR": "$LocalCIDR"}
  ],
  "XrayProfiles": [
    {
      "ID": "xray-client",
      "Enabled": true,
      "Runtime": "$XrayRuntime",
      "OutboundProtocol": "vless",
      "OutboundSettingsJSON": "{\"vnext\":[{\"address\":\"$HostA\",\"port\":$XrayPort,\"users\":[{\"id\":\"$vlessID\",\"encryption\":\"none\"}]}]}",
      "Network": "tcp",
      "Security": "none",
      "StreamSettingsJSON": "{}"
    }
  ],
  "Connectors": [
    {
      "ID": "xray-frame-out",
      "Enabled": true,
      "Remote": "tapx.frame.local",
      "Port": 1,
      "Transport": "xray",
      "XrayProfileID": "xray-client",
      "RawTCP": {"LengthMode": "uint16"},
      "Binding": {"DeviceID": "dev-xray"}
    }
  ],
  "Settings": [
    {"ID":"global","Enabled":true,"ExternalXrayPath":"$RemoteDir/xray","DataDir":"$RemoteDir/data","LogLevel":"warn"}
  ]
}
"@
}

function Invoke-XrayFrameCase {
    param(
        [Parameter(Mandatory = $true)][string]$CaseName,
        [Parameter(Mandatory = $true)][string]$DeviceType,
        [Parameter(Mandatory = $true)][string]$IfName,
        [Parameter(Mandatory = $true)][string]$ServerCIDR,
        [Parameter(Mandatory = $true)][string]$ClientCIDR,
        [Parameter(Mandatory = $true)][string]$ServerIP,
        [Parameter(Mandatory = $true)][string]$ClientIP,
        [Parameter(Mandatory = $true)][string]$XrayRuntime
    )

    $serverConfig = Join-Path $work "$CaseName-a.json"
    $clientConfig = Join-Path $work "$CaseName-b.json"
    Write-TapXTextFile $serverConfig (New-TapXXrayFrameServerConfig -DeviceType $DeviceType -IfName $IfName -LocalCIDR $ServerCIDR -XrayRuntime $XrayRuntime)
    Write-TapXTextFile $clientConfig (New-TapXXrayFrameClientConfig -DeviceType $DeviceType -IfName $IfName -LocalCIDR $ClientCIDR -XrayRuntime $XrayRuntime)

    Copy-TapXToRemote -Context $ctx -HostName $HostA -LocalPath $serverConfig -RemotePath "$RemoteDir/xray-frame.json"
    Copy-TapXToRemote -Context $ctx -HostName $HostB -LocalPath $clientConfig -RemotePath "$RemoteDir/xray-frame.json"

    try {
        Write-Host "start $XrayRuntime Xray frame/$($DeviceType.ToUpperInvariant()) listener"
        Start-TapXRemoteRuntime -Context $ctx -HostName $HostA -ConfigName "xray-frame.json" -LogName "xray-frame.log" -PidName "xray-frame.pid"
        Invoke-TapXRemoteScript -Context $ctx -HostName $HostA -Script @"
set -euo pipefail
ss -ltnp | grep ':$XrayPort'
ip a show dev $IfName
ip -d addr show dev $IfName
"@ | Out-Host

        Write-Host "start $XrayRuntime Xray frame/$($DeviceType.ToUpperInvariant()) connector"
        Start-TapXRemoteRuntime -Context $ctx -HostName $HostB -ConfigName "xray-frame.json" -LogName "xray-frame.log" -PidName "xray-frame.pid"
        Invoke-TapXRemoteScript -Context $ctx -HostName $HostB -Script @"
set -euo pipefail
ip a show dev $IfName
ip -d addr show dev $IfName
"@ | Out-Host

        Write-Host "verify bidirectional $($DeviceType.ToUpperInvariant()) reachability over $XrayRuntime Xray"
        Invoke-TapXSSH -Context $ctx -HostName $HostA -Command "ping -c 3 -W 2 $ClientIP" | Out-Host
        Invoke-TapXSSH -Context $ctx -HostName $HostB -Command "ping -c 3 -W 2 $ServerIP" | Out-Host
        if ($Throughput) {
            Write-Host "measure bidirectional $($DeviceType.ToUpperInvariant()) payload throughput over $XrayRuntime Xray"
            Invoke-TunnelThroughputDirection -ServerHost $HostA -ClientHost $HostB -ServerIP $ServerIP -Port 47901 -Label "$($DeviceType.ToUpperInvariant()) upload B->A"
            Invoke-TunnelThroughputDirection -ServerHost $HostB -ClientHost $HostA -ServerIP $ClientIP -Port 47902 -Label "$($DeviceType.ToUpperInvariant()) download A->B"
        }
        Write-Host "$XrayRuntime Xray frame/$($DeviceType.ToUpperInvariant()) smoke: ok"
    }
    finally {
        try { Stop-TapXRemoteRuntime -Context $ctx -HostName $HostA } catch { Write-Warning $_ }
        try { Stop-TapXRemoteRuntime -Context $ctx -HostName $HostB } catch { Write-Warning $_ }
    }
}

try {
    Write-Host "prepare remote hosts in $RemoteDir"
    if ($UseInstalledBinary) {
        foreach ($hostName in @($HostA, $HostB)) {
            Invoke-TapXRemoteScript -Context $ctx -HostName $hostName -Script @"
set -euo pipefail
mkdir -p '$RemoteDir'
cp /usr/local/bin/tapx-core '$RemoteDir/tapx-core'
chmod 700 '$RemoteDir/tapx-core'
"@ | Out-Null
        }
    } else {
        Prepare-TapXHost -Context $ctx -HostName $HostA
        Prepare-TapXHost -Context $ctx -HostName $HostB
    }

    if ($Runtime -eq "external" -or $Runtime -eq "both") {
        if ([string]::IsNullOrWhiteSpace($XrayDownloadURL)) {
            $resolvedXray = (Resolve-Path (Join-Path $repo $XrayBinaryPath)).Path
            $xrayArchive = Get-TapXCompressedBinary $resolvedXray
            Copy-TapXToRemote -Context $ctx -HostName $HostA -LocalPath $xrayArchive -RemotePath "$RemoteDir/xray.gz"
            Copy-TapXToRemote -Context $ctx -HostName $HostB -LocalPath $xrayArchive -RemotePath "$RemoteDir/xray.gz"
            Invoke-TapXSSH -Context $ctx -HostName $HostA -Command "gzip -dc '$RemoteDir/xray.gz' >'$RemoteDir/xray' && chmod 755 '$RemoteDir/xray' && rm -f '$RemoteDir/xray.gz'"
            Invoke-TapXSSH -Context $ctx -HostName $HostB -Command "gzip -dc '$RemoteDir/xray.gz' >'$RemoteDir/xray' && chmod 755 '$RemoteDir/xray' && rm -f '$RemoteDir/xray.gz'"
        } else {
            foreach ($hostName in @($HostA, $HostB)) {
                Invoke-TapXRemoteScript -Context $ctx -HostName $hostName -Script @"
set -euo pipefail
command -v curl >/dev/null
command -v unzip >/dev/null
curl --fail --location --silent --show-error --retry 3 --output '$RemoteDir/xray.zip' '$XrayDownloadURL'
unzip -p '$RemoteDir/xray.zip' xray >'$RemoteDir/xray'
chmod 755 '$RemoteDir/xray'
rm -f '$RemoteDir/xray.zip'
'$RemoteDir/xray' version | head -n 1
"@ | Out-Host
            }
        }
    }

    $runtimes = if ($Runtime -eq "both") { @("embedded", "external") } else { @($Runtime) }
    foreach ($xrayRuntime in $runtimes) {
        if ($Mode -eq "tun" -or $Mode -eq "both") {
            Invoke-XrayFrameCase `
                -CaseName "xray-frame-$xrayRuntime-tun" `
                -DeviceType "tun" `
                -IfName "tapxxray0" `
                -ServerCIDR "10.79.0.1/30" `
                -ClientCIDR "10.79.0.2/30" `
                -ServerIP "10.79.0.1" `
                -ClientIP "10.79.0.2" `
                -XrayRuntime $xrayRuntime
        }

        if ($Mode -eq "tap" -or $Mode -eq "both") {
            Invoke-XrayFrameCase `
                -CaseName "xray-frame-$xrayRuntime-tap" `
                -DeviceType "tap" `
                -IfName "tapxxraytap0" `
                -ServerCIDR "10.81.0.1/30" `
                -ClientCIDR "10.81.0.2/30" `
                -ServerIP "10.81.0.1" `
                -ClientIP "10.81.0.2" `
                -XrayRuntime $xrayRuntime
        }
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
