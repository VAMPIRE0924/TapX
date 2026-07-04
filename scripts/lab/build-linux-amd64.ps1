param(
    [ValidateSet("auto", "wsl", "docker")]
    [string]$Builder = "auto",
    [string]$Distro = "Ubuntu-24.04",
    [string]$DockerImage = "golang:1.26-bookworm",
    [string]$OutputPath = "build/lab/linux-amd64/tapx-core"
)

$ErrorActionPreference = "Stop"

function Convert-ToWslPath {
    param([Parameter(Mandatory = $true)][string]$Path)
    $resolved = [IO.Path]::GetFullPath($Path)
    if ($resolved -match '^([A-Za-z]):\\(.*)$') {
        $drive = $matches[1].ToLowerInvariant()
        $rest = $matches[2].Replace('\', '/')
        return "/mnt/$drive/$rest"
    }
    return $resolved.Replace('\', '/')
}

function Initialize-Output {
    param([Parameter(Mandatory = $true)][string]$Path)
    $parent = Split-Path -Parent $Path
    New-Item -ItemType Directory -Force -Path $parent | Out-Null
}

function Build-WithWsl {
    param(
        [Parameter(Mandatory = $true)][string]$Repo,
        [Parameter(Mandatory = $true)][string]$OutputHost
    )
    $repoWsl = Convert-ToWslPath $Repo
    $outputWsl = Convert-ToWslPath $OutputHost
    $outputDirWsl = ($outputWsl -replace '/[^/]+$', '')
    wsl -d $Distro -- bash -lc "set -euo pipefail; cd '$repoWsl'; mkdir -p '$outputDirWsl'; GOTOOLCHAIN=local CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o '$outputWsl' ./cmd/tapx-core; file '$outputWsl'"
}

function Build-WithDocker {
    param(
        [Parameter(Mandatory = $true)][string]$Repo,
        [Parameter(Mandatory = $true)][string]$OutputPath
    )
    $repoMount = $Repo
    $outDir = ($OutputPath -replace '\\', '/') -replace '/[^/]+$', ''
    $outFile = $OutputPath.Replace('\', '/')
    $script = @"
set -euo pipefail
mkdir -p '$outDir' build/gocache-linux build/gomodcache
go version
GOTOOLCHAIN=local GOCACHE=/src/build/gocache-linux GOMODCACHE=/src/build/gomodcache CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o '$outFile' ./cmd/tapx-core
file '$outFile'
"@
    & docker run --rm -v "${repoMount}:/src" -w /src $DockerImage bash -lc $script
    if ($LASTEXITCODE -ne 0) {
        throw "docker linux-amd64 build failed"
    }
}

$repo = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$outputHost = Join-Path $repo $OutputPath
Initialize-Output $outputHost

$errors = @()
if ($Builder -eq "docker" -or $Builder -eq "auto") {
    try {
        Build-WithDocker -Repo $repo -OutputPath $OutputPath
        $resolved = Resolve-Path (Join-Path $repo $OutputPath)
        Write-Host "built $resolved"
        return
    } catch {
        if ($Builder -eq "docker") { throw }
        $errors += "docker: $($_.Exception.Message)"
    }
}

if ($Builder -eq "wsl" -or $Builder -eq "auto") {
    try {
        Build-WithWsl -Repo $repo -OutputHost $outputHost
        $resolved = Resolve-Path (Join-Path $repo $OutputPath)
        Write-Host "built $resolved"
        return
    } catch {
        if ($Builder -eq "wsl") { throw }
        $errors += "wsl: $($_.Exception.Message)"
    }
}

throw "linux-amd64 build failed. Tried: $($errors -join '; ')"
