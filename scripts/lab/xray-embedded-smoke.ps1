param(
    [Parameter(Mandatory = $true)]
    [string[]]$HostName,

    [Parameter(Mandatory = $true)]
    [string]$KeyPath,

    [string]$User = "root",
    [string]$BinaryPath = "build/lab/linux-amd64/tapx-core",
    [string]$RemoteDir = "",
    [int]$XrayPort = 18080,
    [switch]$Build,
    [switch]$KeepRemote
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "common.ps1")

if ($XrayPort -lt 1 -or $XrayPort -gt 65535) {
    throw "XrayPort must be between 1 and 65535"
}

$targetHosts = @(
    foreach ($item in $HostName) {
        $item -split "[,\s]+" | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
    }
)
if ($targetHosts.Count -eq 0) {
    throw "HostName must contain at least one host"
}

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

function New-TapXEmbeddedXrayConfig {
    param([int]$Port)
    return @"
{
  "XrayProfiles": [
    {
      "ID": "xray-embedded",
      "Enabled": true,
      "Runtime": "embedded",
      "InboundProtocol": "dokodemo-door",
      "InboundSettingsJSON": "{\"address\":\"127.0.0.1\",\"port\":80,\"network\":\"tcp\"}",
      "Network": "tcp",
      "Security": "none",
      "StreamSettingsJSON": "{}",
      "AdvancedJSON": "{\"outbounds\":[{\"tag\":\"direct\",\"protocol\":\"freedom\"}],\"routing\":{\"rules\":[{\"type\":\"field\",\"inboundTag\":[\"listener-xray-embedded\"],\"outboundTag\":\"direct\"}]}}"
    }
  ],
  "Listeners": [
    {
      "ID": "listener-xray-embedded",
      "Enabled": true,
      "BindHost": "127.0.0.1",
      "BindPort": $Port,
      "Transport": "xray",
      "XrayProfileID": "xray-embedded",
      "Binding": {}
    }
  ]
}
"@
}

$configPath = Join-Path $work "xray-embedded.json"
Write-TapXTextFile $configPath (New-TapXEmbeddedXrayConfig -Port $XrayPort)

try {
    foreach ($targetHost in $targetHosts) {
        Write-Host "prepare $targetHost in $RemoteDir"
        Prepare-TapXHost -Context $ctx -HostName $targetHost
        Copy-TapXToRemote -Context $ctx -HostName $targetHost -LocalPath $configPath -RemotePath "$RemoteDir/xray-embedded.json"
        Start-TapXRemoteRuntime -Context $ctx -HostName $targetHost -ConfigName "xray-embedded.json" -LogName "xray-embedded.log" -PidName "xray-embedded.pid"

        Write-Host "$targetHost embedded Xray listener"
        Invoke-TapXRemoteScript -Context $ctx -HostName $targetHost -Script @"
set -euo pipefail
ss -ltnp | grep ':$XrayPort' || {
  cat '$RemoteDir/xray-embedded.log' >&2 || true
  exit 1
}
if pgrep -x xray >/dev/null 2>&1; then
  echo 'external_xray_process=present' >&2
  exit 1
fi
echo 'external_xray_process=absent'
"@ | Out-Host

        Stop-TapXRemoteRuntime -Context $ctx -HostName $targetHost
    }
    Write-Host "embedded Xray smoke: ok"
}
finally {
    if ($KeepRemote) {
        Write-Host "keeping remote lab directory: $RemoteDir"
    } else {
        Write-Host "cleanup remote lab directory"
        foreach ($targetHost in $targetHosts) {
            try { Stop-TapXRemoteRuntime -Context $ctx -HostName $targetHost } catch { Write-Warning $_ }
            try { Remove-TapXRemoteDir -Context $ctx -HostName $targetHost } catch { Write-Warning $_ }
        }
    }
    Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
}
