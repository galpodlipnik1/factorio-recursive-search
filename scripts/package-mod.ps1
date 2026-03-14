param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# Approved direction: package the repository into a Factorio-ready zip named from info.json
# by staging the repo contents under <name>_<version>, then move that zip to %APPDATA%\Factorio\mods.

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$infoPath = Join-Path $repoRoot 'info.json'

if (-not (Test-Path -LiteralPath $infoPath)) {
    throw "Missing info.json at '$infoPath'."
}

$info = Get-Content -LiteralPath $infoPath -Raw | ConvertFrom-Json

if ([string]::IsNullOrWhiteSpace($info.name) -or [string]::IsNullOrWhiteSpace($info.version)) {
    throw "info.json must define non-empty 'name' and 'version' fields."
}

$packageName = '{0}_{1}' -f $info.name, $info.version
$modsDir = Join-Path $env:APPDATA 'Factorio\mods'
$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ('factorio-mod-package-' + [System.Guid]::NewGuid().ToString('N'))
$stageRoot = Join-Path $tempRoot $packageName
$zipPath = Join-Path $tempRoot ($packageName + '.zip')
$destinationZip = Join-Path $modsDir ($packageName + '.zip')

New-Item -ItemType Directory -Path $stageRoot -Force | Out-Null
New-Item -ItemType Directory -Path $modsDir -Force | Out-Null

try {
    Get-ChildItem -LiteralPath $repoRoot -Force | Where-Object { $_.Name -ne '.git' } | ForEach-Object {
        Copy-Item -LiteralPath $_.FullName -Destination $stageRoot -Recurse -Force
    }

    if (Test-Path -LiteralPath $zipPath) {
        Remove-Item -LiteralPath $zipPath -Force
    }

    Compress-Archive -LiteralPath $stageRoot -DestinationPath $zipPath -CompressionLevel Optimal

    if (Test-Path -LiteralPath $destinationZip) {
        Remove-Item -LiteralPath $destinationZip -Force
    }

    Move-Item -LiteralPath $zipPath -Destination $destinationZip -Force
    Write-Host "Created $destinationZip"
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}
