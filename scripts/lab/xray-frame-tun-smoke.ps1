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

function New-TapXXrayFrameServerConfig {
    param(
        [Parameter(Mandatory = $true)][string]$DeviceType,
        [Parameter(Mandatory = $true)][string]$IfName,
        [Parameter(Mandatory = $true)][string]$LocalCIDR
    )
    return @"
{
  "Devices": [
    {"ID": "dev-xray", "Enabled": true, "Type": "$DeviceType", "IfName": "$IfName", "MTU": 1400, "IPv4CIDR": "$LocalCIDR"}
  ],
  "XrayProfiles": [
    {
      "ID": "xray-server",
      "Enabled": true,
      "Runtime": "embedded",
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
  ]
}
"@
}

function New-TapXXrayFrameClientConfig {
    param(
        [Parameter(Mandatory = $true)][string]$DeviceType,
        [Parameter(Mandatory = $true)][string]$IfName,
        [Parameter(Mandatory = $true)][string]$LocalCIDR
    )
    return @"
{
  "Devices": [
    {"ID": "dev-xray", "Enabled": true, "Type": "$DeviceType", "IfName": "$IfName", "MTU": 1400, "IPv4CIDR": "$LocalCIDR"}
  ],
  "XrayProfiles": [
    {
      "ID": "xray-client",
      "Enabled": true,
      "Runtime": "embedded",
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
        [Parameter(Mandatory = $true)][string]$ClientIP
    )

    $serverConfig = Join-Path $work "$CaseName-a.json"
    $clientConfig = Join-Path $work "$CaseName-b.json"
    Write-TapXTextFile $serverConfig (New-TapXXrayFrameServerConfig -DeviceType $DeviceType -IfName $IfName -LocalCIDR $ServerCIDR)
    Write-TapXTextFile $clientConfig (New-TapXXrayFrameClientConfig -DeviceType $DeviceType -IfName $IfName -LocalCIDR $ClientCIDR)

    Copy-TapXToRemote -Context $ctx -HostName $HostA -LocalPath $serverConfig -RemotePath "$RemoteDir/xray-frame.json"
    Copy-TapXToRemote -Context $ctx -HostName $HostB -LocalPath $clientConfig -RemotePath "$RemoteDir/xray-frame.json"

    try {
        Write-Host "start embedded Xray frame/$($DeviceType.ToUpperInvariant()) listener"
        Start-TapXRemoteRuntime -Context $ctx -HostName $HostA -ConfigName "xray-frame.json" -LogName "xray-frame.log" -PidName "xray-frame.pid"
        Invoke-TapXRemoteScript -Context $ctx -HostName $HostA -Script @"
set -euo pipefail
ss -ltnp | grep ':$XrayPort'
ip a show dev $IfName
ip -d addr show dev $IfName
"@ | Out-Host

        Write-Host "start embedded Xray frame/$($DeviceType.ToUpperInvariant()) connector"
        Start-TapXRemoteRuntime -Context $ctx -HostName $HostB -ConfigName "xray-frame.json" -LogName "xray-frame.log" -PidName "xray-frame.pid"
        Invoke-TapXRemoteScript -Context $ctx -HostName $HostB -Script @"
set -euo pipefail
ip a show dev $IfName
ip -d addr show dev $IfName
"@ | Out-Host

        Write-Host "verify bidirectional $($DeviceType.ToUpperInvariant()) reachability over embedded Xray"
        Invoke-TapXSSH -Context $ctx -HostName $HostA -Command "ping -c 3 -W 2 $ClientIP" | Out-Host
        Invoke-TapXSSH -Context $ctx -HostName $HostB -Command "ping -c 3 -W 2 $ServerIP" | Out-Host
        Write-Host "embedded Xray frame/$($DeviceType.ToUpperInvariant()) smoke: ok"
    }
    finally {
        try { Stop-TapXRemoteRuntime -Context $ctx -HostName $HostA } catch { Write-Warning $_ }
        try { Stop-TapXRemoteRuntime -Context $ctx -HostName $HostB } catch { Write-Warning $_ }
    }
}

try {
    Write-Host "prepare remote hosts in $RemoteDir"
    Prepare-TapXHost -Context $ctx -HostName $HostA
    Prepare-TapXHost -Context $ctx -HostName $HostB

    if ($Mode -eq "tun" -or $Mode -eq "both") {
        Invoke-XrayFrameCase `
            -CaseName "xray-frame-tun" `
            -DeviceType "tun" `
            -IfName "tapxxray0" `
            -ServerCIDR "10.79.0.1/30" `
            -ClientCIDR "10.79.0.2/30" `
            -ServerIP "10.79.0.1" `
            -ClientIP "10.79.0.2"
    }

    if ($Mode -eq "tap" -or $Mode -eq "both") {
        Invoke-XrayFrameCase `
            -CaseName "xray-frame-tap" `
            -DeviceType "tap" `
            -IfName "tapxxraytap0" `
            -ServerCIDR "10.81.0.1/30" `
            -ClientCIDR "10.81.0.2/30" `
            -ServerIP "10.81.0.1" `
            -ClientIP "10.81.0.2"
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
