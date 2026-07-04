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
    [int]$UdpPort = 46000,
    [int]$TcpPort = 46001,
    [int]$TapPort = 46002,
    [ValidateSet("udp", "tcp", "tap", "all", "both")]
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

$udpA = Join-Path $work "udp-a.json"
$udpB = Join-Path $work "udp-b.json"
$tcpA = Join-Path $work "tcp-a.json"
$tcpB = Join-Path $work "tcp-b.json"
$tapA = Join-Path $work "tap-a.json"
$tapB = Join-Path $work "tap-b.json"
Write-TapXTextFile $udpA (New-TapXUdpConfig -LocalIP "10.77.0.1" -PeerHost $HostB -UdpPort $UdpPort)
Write-TapXTextFile $udpB (New-TapXUdpConfig -LocalIP "10.77.0.2" -PeerHost $HostA -UdpPort $UdpPort)
Write-TapXTextFile $tcpA (New-TapXTcpListenerConfig -TcpPort $TcpPort)
Write-TapXTextFile $tcpB (New-TapXTcpConnectorConfig -HostA $HostA -TcpPort $TcpPort)
Write-TapXTextFile $tapA (New-TapXTapUdpConfig -LocalIP "10.80.0.1" -PeerHost $HostB -UdpPort $TapPort)
Write-TapXTextFile $tapB (New-TapXTapUdpConfig -LocalIP "10.80.0.2" -PeerHost $HostA -UdpPort $TapPort)

try {
    Write-Host "prepare remote hosts in $RemoteDir"
    Prepare-TapXHost -Context $ctx -HostName $HostA
    Prepare-TapXHost -Context $ctx -HostName $HostB

    if ($Mode -eq "udp" -or $Mode -eq "both" -or $Mode -eq "all") {
        Write-Host "run raw UDP/TUN smoke"
        Copy-TapXToRemote -Context $ctx -HostName $HostA -LocalPath $udpA -RemotePath "$RemoteDir/udp.json"
        Copy-TapXToRemote -Context $ctx -HostName $HostB -LocalPath $udpB -RemotePath "$RemoteDir/udp.json"
        Start-TapXRemoteRuntime -Context $ctx -HostName $HostA -ConfigName "udp.json" -LogName "udp.log" -PidName "udp.pid"
        Start-TapXRemoteRuntime -Context $ctx -HostName $HostB -ConfigName "udp.json" -LogName "udp.log" -PidName "udp.pid"
        Write-Host "HostA UDP TUN interface"
        Invoke-TapXSSH -Context $ctx -HostName $HostA -Command "ip -d addr show dev tapxudp0" | Out-Host
        Write-Host "HostB UDP TUN interface"
        Invoke-TapXSSH -Context $ctx -HostName $HostB -Command "ip -d addr show dev tapxudp0" | Out-Host
        Invoke-TapXSSH -Context $ctx -HostName $HostA -Command "ping -c 3 -W 2 10.77.0.2" | Out-Host
        Invoke-TapXSSH -Context $ctx -HostName $HostB -Command "ping -c 3 -W 2 10.77.0.1" | Out-Host
        Stop-TapXRemoteRuntime -Context $ctx -HostName $HostA
        Stop-TapXRemoteRuntime -Context $ctx -HostName $HostB
        Write-Host "raw UDP/TUN smoke: ok"
    }

    if ($Mode -eq "tcp" -or $Mode -eq "both" -or $Mode -eq "all") {
        Write-Host "run raw TCP/TUN smoke"
        Copy-TapXToRemote -Context $ctx -HostName $HostA -LocalPath $tcpA -RemotePath "$RemoteDir/tcp.json"
        Copy-TapXToRemote -Context $ctx -HostName $HostB -LocalPath $tcpB -RemotePath "$RemoteDir/tcp.json"
        Start-TapXRemoteRuntime -Context $ctx -HostName $HostA -ConfigName "tcp.json" -LogName "tcp.log" -PidName "tcp.pid"
        Start-TapXRemoteRuntime -Context $ctx -HostName $HostB -ConfigName "tcp.json" -LogName "tcp.log" -PidName "tcp.pid"
        Write-Host "HostA TCP TUN interface"
        Invoke-TapXSSH -Context $ctx -HostName $HostA -Command "ip -d addr show dev tapxtcp0" | Out-Host
        Write-Host "HostB TCP TUN interface"
        Invoke-TapXSSH -Context $ctx -HostName $HostB -Command "ip -d addr show dev tapxtcp0" | Out-Host
        Invoke-TapXSSH -Context $ctx -HostName $HostA -Command "ping -c 3 -W 2 10.78.0.2" | Out-Host
        Invoke-TapXSSH -Context $ctx -HostName $HostB -Command "ping -c 3 -W 2 10.78.0.1" | Out-Host
        Stop-TapXRemoteRuntime -Context $ctx -HostName $HostA
        Stop-TapXRemoteRuntime -Context $ctx -HostName $HostB
        Write-Host "raw TCP/TUN smoke: ok"
    }

    if ($Mode -eq "tap" -or $Mode -eq "all") {
        Write-Host "run raw UDP/TAP smoke"
        Copy-TapXToRemote -Context $ctx -HostName $HostA -LocalPath $tapA -RemotePath "$RemoteDir/tap.json"
        Copy-TapXToRemote -Context $ctx -HostName $HostB -LocalPath $tapB -RemotePath "$RemoteDir/tap.json"
        Start-TapXRemoteRuntime -Context $ctx -HostName $HostA -ConfigName "tap.json" -LogName "tap.log" -PidName "tap.pid"
        Start-TapXRemoteRuntime -Context $ctx -HostName $HostB -ConfigName "tap.json" -LogName "tap.log" -PidName "tap.pid"
        Write-Host "HostA TAP interface"
        Invoke-TapXSSH -Context $ctx -HostName $HostA -Command "ip -d addr show dev tapxtap0" | Out-Host
        Write-Host "HostB TAP interface"
        Invoke-TapXSSH -Context $ctx -HostName $HostB -Command "ip -d addr show dev tapxtap0" | Out-Host
        Invoke-TapXSSH -Context $ctx -HostName $HostA -Command "ping -c 3 -W 2 10.80.0.2" | Out-Host
        Invoke-TapXSSH -Context $ctx -HostName $HostB -Command "ping -c 3 -W 2 10.80.0.1" | Out-Host
        Stop-TapXRemoteRuntime -Context $ctx -HostName $HostA
        Stop-TapXRemoteRuntime -Context $ctx -HostName $HostB
        Write-Host "raw UDP/TAP smoke: ok"
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
