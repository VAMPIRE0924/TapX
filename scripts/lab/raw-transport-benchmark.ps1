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
    [int]$UdpPort = 46100,
    [int]$TcpPort = 46101,
    [int]$IperfPort = 5201,
    [ValidateSet("udp", "tcp", "both")]
    [string]$Mode = "both",
    [ValidateSet("tcp", "udp", "both")]
    [string]$Traffic = "tcp",
    [ValidateSet("auto", "iperf3", "python")]
    [string]$Tool = "auto",
    [int]$Duration = 20,
    [int]$Parallel = 1,
    [string]$UdpBitrate = "0",
    [string]$OutputDir = "",
    [switch]$Build,
    [switch]$KeepRemote
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "common.ps1")

if ($Duration -lt 1) {
    throw "Duration must be >= 1"
}
if ($Parallel -lt 1) {
    throw "Parallel must be >= 1"
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

if (-not [string]::IsNullOrWhiteSpace($OutputDir)) {
    New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
}

$udpA = Join-Path $work "udp-a.json"
$udpB = Join-Path $work "udp-b.json"
$tcpA = Join-Path $work "tcp-a.json"
$tcpB = Join-Path $work "tcp-b.json"
Write-TapXTextFile $udpA (New-TapXUdpConfig -LocalIP "10.77.0.1" -PeerHost $HostB -UdpPort $UdpPort)
Write-TapXTextFile $udpB (New-TapXUdpConfig -LocalIP "10.77.0.2" -PeerHost $HostA -UdpPort $UdpPort)
Write-TapXTextFile $tcpA (New-TapXTcpListenerConfig -TcpPort $TcpPort)
Write-TapXTextFile $tcpB (New-TapXTcpConnectorConfig -HostA $HostA -TcpPort $TcpPort)

function Get-BenchmarkToolStatus {
    param([string]$HostName)
    $lines = Invoke-TapXSSH -Context $ctx -HostName $HostName -Command "if command -v iperf3 >/dev/null 2>&1; then echo iperf3=yes; else echo iperf3=no; fi; if command -v python3 >/dev/null 2>&1; then echo python3=yes; else echo python3=no; fi"
    $status = @{ iperf3 = $false; python3 = $false }
    foreach ($line in $lines) {
        if ($line -match "^iperf3=(yes|no)$") {
            $status.iperf3 = ($Matches[1] -eq "yes")
        } elseif ($line -match "^python3=(yes|no)$") {
            $status.python3 = ($Matches[1] -eq "yes")
        }
    }
    return $status
}

function Resolve-BenchmarkTool {
    $a = Get-BenchmarkToolStatus -HostName $HostA
    $b = Get-BenchmarkToolStatus -HostName $HostB
    Write-Host "HostA tools: iperf3=$($a.iperf3) python3=$($a.python3)"
    Write-Host "HostB tools: iperf3=$($b.iperf3) python3=$($b.python3)"

    if ($Tool -eq "iperf3") {
        if (-not ($a.iperf3 -and $b.iperf3)) {
            throw "Tool=iperf3 requires iperf3 on both hosts"
        }
        return "iperf3"
    }
    if ($Tool -eq "python") {
        if (-not ($a.python3 -and $b.python3)) {
            throw "Tool=python requires python3 on both hosts"
        }
        return "python"
    }
    if ($a.iperf3 -and $b.iperf3) {
        return "iperf3"
    }
    if ($a.python3 -and $b.python3) {
        return "python"
    }
    throw "No supported benchmark tool found on both hosts. Install/provide iperf3 or python3."
}

function Save-BenchmarkJSON {
    param(
        [string]$Name,
        [string]$TrafficMode,
        [string]$ToolName,
        [string]$Role,
        [string]$Text
    )
    if ([string]::IsNullOrWhiteSpace($OutputDir)) {
        return
    }
    $path = Join-Path $OutputDir "$runId-$Name-$TrafficMode-$ToolName-$Role.json"
    Write-TapXTextFile $path $Text
}

function Format-Mbps {
    param([double]$BitsPerSecond)
    return [Math]::Round($BitsPerSecond / 1000000, 2)
}

function Start-IperfServer {
    param(
        [string]$HostName,
        [string]$BindIP,
        [int]$Port,
        [string]$Name
    )
    Invoke-TapXRemoteScript -Context $ctx -HostName $HostName -Script @"
set -euo pipefail
cd '$RemoteDir'
rm -f 'iperf-$Name.log' 'iperf-$Name.pid'
nohup iperf3 -s -B '$BindIP' -p '$Port' --one-off >'iperf-$Name.log' 2>&1 &
echo `$! >'iperf-$Name.pid'
sleep 0.5
if ! kill -0 `$(cat 'iperf-$Name.pid') >/dev/null 2>&1; then
  cat 'iperf-$Name.log' >&2 || true
  exit 1
fi
"@ | Out-Host
}

function Invoke-IperfClient {
    param(
        [string]$HostName,
        [string]$ServerIP,
        [int]$Port,
        [string]$TrafficMode,
        [string]$Name
    )
    $udpArgs = ""
    if ($TrafficMode -eq "udp") {
        $udpArgs = "-u -b '$UdpBitrate'"
    }
    $json = Invoke-TapXSSH -Context $ctx -HostName $HostName -Command "iperf3 -J $udpArgs -c '$ServerIP' -p '$Port' -t '$Duration' -P '$Parallel'"
    $text = ($json -join "`n")
    Save-BenchmarkJSON -Name $Name -TrafficMode $TrafficMode -ToolName "iperf3" -Role "client" -Text $text
    $decoded = $text | ConvertFrom-Json
    $bitsPerSecond = $null
    if ($null -ne $decoded.end.sum_received.bits_per_second) {
        $bitsPerSecond = [double]$decoded.end.sum_received.bits_per_second
    } elseif ($null -ne $decoded.end.sum.bits_per_second) {
        $bitsPerSecond = [double]$decoded.end.sum.bits_per_second
    }
    if ($null -ne $bitsPerSecond) {
        Write-Host "$Name $TrafficMode iperf3 throughput: $(Format-Mbps $bitsPerSecond) Mbit/s"
    } else {
        Write-Host "$Name $TrafficMode iperf3 finished"
    }
}

function Start-PythonBenchServer {
    param(
        [string]$HostName,
        [string]$BindIP,
        [int]$Port,
        [string]$TrafficMode,
        [string]$Name
    )
    $serverDuration = $Duration + 4
    Invoke-TapXRemoteScript -Context $ctx -HostName $HostName -Script @"
set -euo pipefail
cd '$RemoteDir'
cat > 'pybench-server-$Name.py' <<'PY'
import argparse
import json
import selectors
import socket
import sys
import time


def tcp_server(args):
    total = 0
    accepted = 0
    start = time.monotonic()
    deadline = start + args.duration
    sel = selectors.DefaultSelector()
    srv = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    srv.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    srv.bind((args.bind, args.port))
    srv.listen(max(args.parallel, 1))
    srv.setblocking(False)
    sel.register(srv, selectors.EVENT_READ, "listener")
    try:
        while time.monotonic() < deadline:
            events = sel.select(min(0.5, max(0.0, deadline - time.monotonic())))
            if not events:
                if accepted >= args.parallel and len(sel.get_map()) == 1:
                    break
                continue
            for key, _ in events:
                if key.data == "listener":
                    conn, _ = srv.accept()
                    conn.setblocking(False)
                    accepted += 1
                    sel.register(conn, selectors.EVENT_READ, "conn")
                    continue
                try:
                    data = key.fileobj.recv(65536)
                except BlockingIOError:
                    continue
                if data:
                    total += len(data)
                else:
                    sel.unregister(key.fileobj)
                    key.fileobj.close()
            if accepted >= args.parallel and len(sel.get_map()) == 1:
                break
    finally:
        for key in list(sel.get_map().values()):
            try:
                sel.unregister(key.fileobj)
            except Exception:
                pass
            try:
                key.fileobj.close()
            except Exception:
                pass
        sel.close()
    elapsed = max(time.monotonic() - start, 0.001)
    return {"tool": "python", "role": "server", "mode": "tcp", "bytes_received": total, "elapsed": elapsed, "bits_per_second": total * 8 / elapsed}


def udp_server(args):
    total = 0
    packets = 0
    start = time.monotonic()
    deadline = start + args.duration
    last_rx = None
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.bind((args.bind, args.port))
    sock.settimeout(0.5)
    try:
        while time.monotonic() < deadline:
            try:
                data, _ = sock.recvfrom(65535)
            except socket.timeout:
                if last_rx is not None and time.monotonic() - last_rx > 1.0:
                    break
                continue
            total += len(data)
            packets += 1
            last_rx = time.monotonic()
    finally:
        sock.close()
    elapsed = max(time.monotonic() - start, 0.001)
    return {"tool": "python", "role": "server", "mode": "udp", "bytes_received": total, "packets_received": packets, "elapsed": elapsed, "bits_per_second": total * 8 / elapsed}


parser = argparse.ArgumentParser()
parser.add_argument("--mode", choices=["tcp", "udp"], required=True)
parser.add_argument("--bind", required=True)
parser.add_argument("--port", type=int, required=True)
parser.add_argument("--duration", type=float, required=True)
parser.add_argument("--parallel", type=int, default=1)
args = parser.parse_args()

try:
    result = tcp_server(args) if args.mode == "tcp" else udp_server(args)
    print(json.dumps(result, separators=(",", ":")), flush=True)
except Exception as exc:
    print("pybench server error: %s" % exc, file=sys.stderr, flush=True)
    raise
PY
rm -f 'pybench-$Name-server.log' 'pybench-$Name.pid'
nohup python3 'pybench-server-$Name.py' --mode '$TrafficMode' --bind '$BindIP' --port '$Port' --duration '$serverDuration' --parallel '$Parallel' >'pybench-$Name-server.log' 2>&1 &
echo `$! >'pybench-$Name.pid'
sleep 0.5
if ! kill -0 `$(cat 'pybench-$Name.pid') >/dev/null 2>&1; then
  cat 'pybench-$Name-server.log' >&2 || true
  exit 1
fi
"@ | Out-Host
}

function Invoke-PythonBenchClient {
    param(
        [string]$HostName,
        [string]$ServerIP,
        [int]$Port,
        [string]$TrafficMode,
        [string]$Name
    )
    $output = Invoke-TapXRemoteScript -Context $ctx -HostName $HostName -Script @"
set -euo pipefail
cd '$RemoteDir'
cat > 'pybench-client-$Name-$TrafficMode.py' <<'PY'
import argparse
import json
import socket
import sys
import threading
import time


def parse_bitrate(value):
    if value is None:
        return 0.0
    text = str(value).strip().lower()
    if text in ("", "0", "unlimited"):
        return 0.0
    scale = 1.0
    for suffix, factor in (("gbps", 1_000_000_000), ("gbit", 1_000_000_000), ("g", 1_000_000_000), ("mbps", 1_000_000), ("mbit", 1_000_000), ("m", 1_000_000), ("kbps", 1_000), ("kbit", 1_000), ("k", 1_000), ("bps", 1)):
        if text.endswith(suffix):
            scale = factor
            text = text[:-len(suffix)]
            break
    return float(text) * scale


def tcp_worker(args, deadline, index, totals, errors):
    payload = b"x" * 65536
    sent = 0
    try:
        sock = socket.create_connection((args.host, args.port), timeout=5)
        sock.setsockopt(socket.IPPROTO_TCP, socket.TCP_NODELAY, 1)
        with sock:
            while time.monotonic() < deadline:
                sock.sendall(payload)
                sent += len(payload)
    except (BrokenPipeError, ConnectionResetError) as exc:
        if sent == 0 and time.monotonic() < deadline:
            errors[index] = str(exc)
    except Exception as exc:
        errors[index] = str(exc)
    totals[index] = sent


def udp_worker(args, deadline, index, totals, errors):
    payload = b"x" * args.udp_size
    sent = 0
    packets = 0
    bps = parse_bitrate(args.bitrate)
    per_thread_bps = bps / max(args.parallel, 1) if bps > 0 else 0.0
    interval = (len(payload) * 8 / per_thread_bps) if per_thread_bps > 0 else 0.0
    next_send = time.monotonic()
    try:
        sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        sock.connect((args.host, args.port))
        with sock:
            while time.monotonic() < deadline:
                sock.send(payload)
                sent += len(payload)
                packets += 1
                if interval > 0:
                    next_send += interval
                    delay = next_send - time.monotonic()
                    if delay > 0:
                        time.sleep(delay)
    except Exception as exc:
        errors[index] = str(exc)
    totals[index] = sent
    return packets


parser = argparse.ArgumentParser()
parser.add_argument("--mode", choices=["tcp", "udp"], required=True)
parser.add_argument("--host", required=True)
parser.add_argument("--port", type=int, required=True)
parser.add_argument("--duration", type=float, required=True)
parser.add_argument("--parallel", type=int, default=1)
parser.add_argument("--bitrate", default="0")
parser.add_argument("--udp-size", type=int, default=1200)
args = parser.parse_args()

start = time.monotonic()
deadline = start + args.duration
totals = [0 for _ in range(args.parallel)]
errors = ["" for _ in range(args.parallel)]
threads = []
for i in range(args.parallel):
    target = tcp_worker if args.mode == "tcp" else udp_worker
    thread = threading.Thread(target=target, args=(args, deadline, i, totals, errors))
    thread.start()
    threads.append(thread)
for thread in threads:
    thread.join()
elapsed = max(time.monotonic() - start, 0.001)
result = {
    "tool": "python",
    "role": "client",
    "mode": args.mode,
    "bytes_sent": sum(totals),
    "elapsed": elapsed,
    "bits_per_second": sum(totals) * 8 / elapsed,
    "parallel": args.parallel,
    "errors": [err for err in errors if err],
}
print(json.dumps(result, separators=(",", ":")), flush=True)
if result["errors"]:
    print("pybench client errors: %s" % result["errors"], file=sys.stderr, flush=True)
PY
python3 'pybench-client-$Name-$TrafficMode.py' --mode '$TrafficMode' --host '$ServerIP' --port '$Port' --duration '$Duration' --parallel '$Parallel' --bitrate '$UdpBitrate'
"@
    $text = ($output -join "`n")
    Save-BenchmarkJSON -Name $Name -TrafficMode $TrafficMode -ToolName "python" -Role "client" -Text $text
    return ($text | ConvertFrom-Json)
}

function Read-PythonBenchServer {
    param(
        [string]$HostName,
        [string]$TrafficMode,
        [string]$Name
    )
    $waitLoops = [Math]::Max(10, ($Duration + 8) * 2)
    $output = Invoke-TapXRemoteScript -Context $ctx -HostName $HostName -Script @"
set -euo pipefail
cd '$RemoteDir'
pid=`$(cat 'pybench-$Name.pid')
for i in `$(seq 1 $waitLoops); do
  if ! kill -0 "`$pid" >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
done
if kill -0 "`$pid" >/dev/null 2>&1; then
  kill -TERM "`$pid" >/dev/null 2>&1 || true
  sleep 0.2
fi
cat 'pybench-$Name-server.log'
"@
    $text = ($output -join "`n")
    Save-BenchmarkJSON -Name $Name -TrafficMode $TrafficMode -ToolName "python" -Role "server" -Text $text
    return ($text | ConvertFrom-Json)
}

function Invoke-PythonBench {
    param(
        [string]$HostName,
        [string]$ServerHostName,
        [string]$ServerIP,
        [int]$Port,
        [string]$TrafficMode,
        [string]$Name
    )
    $benchName = "$Name-$TrafficMode"
    Start-PythonBenchServer -HostName $ServerHostName -BindIP $ServerIP -Port $Port -TrafficMode $TrafficMode -Name $benchName
    $client = Invoke-PythonBenchClient -HostName $HostName -ServerIP $ServerIP -Port $Port -TrafficMode $TrafficMode -Name $Name
    $server = Read-PythonBenchServer -HostName $ServerHostName -TrafficMode $TrafficMode -Name $benchName
    $serverMbps = Format-Mbps ([double]$server.bits_per_second)
    $clientMbps = Format-Mbps ([double]$client.bits_per_second)
    Write-Host "$Name $TrafficMode python throughput: $serverMbps Mbit/s received, $clientMbps Mbit/s sent"
}

function Invoke-BenchmarkPair {
    param(
        [string]$Name,
        [string]$ConfigA,
        [string]$ConfigB,
        [string]$ServerIP,
        [string]$PingIP,
        [string]$InterfaceName,
        [string]$ToolName
    )
    Copy-TapXToRemote -Context $ctx -HostName $HostA -LocalPath $ConfigA -RemotePath "$RemoteDir/$Name-a.json"
    Copy-TapXToRemote -Context $ctx -HostName $HostB -LocalPath $ConfigB -RemotePath "$RemoteDir/$Name-b.json"
    Start-TapXRemoteRuntime -Context $ctx -HostName $HostA -ConfigName "$Name-a.json" -LogName "$Name-a.log" -PidName "$Name-a.pid"
    Start-TapXRemoteRuntime -Context $ctx -HostName $HostB -ConfigName "$Name-b.json" -LogName "$Name-b.log" -PidName "$Name-b.pid"
    Write-Host "HostA $InterfaceName interface"
    Invoke-TapXSSH -Context $ctx -HostName $HostA -Command "ip -d addr show dev '$InterfaceName'" | Out-Host
    Write-Host "HostB $InterfaceName interface"
    Invoke-TapXSSH -Context $ctx -HostName $HostB -Command "ip -d addr show dev '$InterfaceName'" | Out-Host
    Invoke-TapXSSH -Context $ctx -HostName $HostA -Command "ping -c 3 -W 2 '$PingIP'" | Out-Host

    if ($Traffic -eq "tcp" -or $Traffic -eq "both") {
        if ($ToolName -eq "iperf3") {
            Start-IperfServer -HostName $HostB -BindIP $ServerIP -Port $IperfPort -Name "$Name-tcp"
            Invoke-IperfClient -HostName $HostA -ServerIP $ServerIP -Port $IperfPort -TrafficMode "tcp" -Name $Name
        } else {
            Invoke-PythonBench -HostName $HostA -ServerHostName $HostB -ServerIP $ServerIP -Port $IperfPort -TrafficMode "tcp" -Name $Name
        }
    }
    if ($Traffic -eq "udp" -or $Traffic -eq "both") {
        if ($ToolName -eq "iperf3") {
            Start-IperfServer -HostName $HostB -BindIP $ServerIP -Port $IperfPort -Name "$Name-udp"
            Invoke-IperfClient -HostName $HostA -ServerIP $ServerIP -Port $IperfPort -TrafficMode "udp" -Name $Name
        } else {
            Invoke-PythonBench -HostName $HostA -ServerHostName $HostB -ServerIP $ServerIP -Port $IperfPort -TrafficMode "udp" -Name $Name
        }
    }

    Stop-TapXRemoteRuntime -Context $ctx -HostName $HostA
    Stop-TapXRemoteRuntime -Context $ctx -HostName $HostB
}

try {
    Write-Host "prepare remote hosts in $RemoteDir"
    Prepare-TapXHost -Context $ctx -HostName $HostA
    Prepare-TapXHost -Context $ctx -HostName $HostB
    $selectedTool = Resolve-BenchmarkTool
    Write-Host "benchmark tool: $selectedTool"

    if ($Mode -eq "udp" -or $Mode -eq "both") {
        Write-Host "benchmark raw UDP/TUN transport"
        Invoke-BenchmarkPair -Name "raw-udp" -ConfigA $udpA -ConfigB $udpB -ServerIP "10.77.0.2" -PingIP "10.77.0.2" -InterfaceName "tapxudp0" -ToolName $selectedTool
    }
    if ($Mode -eq "tcp" -or $Mode -eq "both") {
        Write-Host "benchmark raw TCP/TUN transport"
        Invoke-BenchmarkPair -Name "raw-tcp" -ConfigA $tcpA -ConfigB $tcpB -ServerIP "10.78.0.2" -PingIP "10.78.0.2" -InterfaceName "tapxtcp0" -ToolName $selectedTool
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
