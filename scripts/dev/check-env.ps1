$ErrorActionPreference = "Stop"

function Need-Command($Name) {
    $cmd = Get-Command $Name -ErrorAction SilentlyContinue
    if (-not $cmd) {
        throw "Missing required Windows command: $Name"
    }
    return $cmd.Source
}

Write-Host "Windows tools:"
Write-Host "git: $(Need-Command git)"
git --version
Write-Host "ssh: $(Need-Command ssh)"
ssh -V
Write-Host "scp: $(Need-Command scp)"
Write-Host "wsl: $(Need-Command wsl)"
wsl -l -v

if (Get-Command go -ErrorAction SilentlyContinue) {
    Write-Host "Windows Go:"
    go version
}

Write-Host "WSL Ubuntu check:"
$repoPath = (Get-Location).Path
$drive = $repoPath.Substring(0, 1).ToLowerInvariant()
$rest = $repoPath.Substring(2).Replace("\", "/")
$wslPath = "/mnt/$drive$rest"
$escaped = $wslPath.Replace("'", "'\''")
wsl -d Ubuntu-24.04 -- bash -lc "cd '$escaped' && ./scripts/dev/check-env.sh"
