param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$info = Get-Content -LiteralPath (Join-Path $repoRoot 'info.json') -Raw | ConvertFrom-Json

$name = [string]$info.name
$version = [string]$info.version
if ([string]::IsNullOrWhiteSpace($name) -or [string]::IsNullOrWhiteSpace($version)) {
    throw 'info.json must contain non-empty name and version.'
}

$packageName = '{0}_{1}' -f $name, $version
$outPath = Join-Path $repoRoot ($packageName + '.zip')
$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ('rbf-package-' + [guid]::NewGuid().ToString('N'))
$stageRoot = Join-Path $tempRoot $packageName

$excludedRoots = @('server', 'deploy', '.github', 'tooling', '.git')

New-Item -ItemType Directory -Path $stageRoot -Force | Out-Null

try {
    Get-ChildItem -LiteralPath $repoRoot -Force | ForEach-Object {
        $item = $_
        if ($excludedRoots -contains $item.Name) {
            return
        }

        if ($item.Name -like '*.zip') {
            return
        }

        Copy-Item -LiteralPath $item.FullName -Destination $stageRoot -Recurse -Force
    }

    $generatedIndex = Join-Path $stageRoot 'generated\index.lua'
    if (Test-Path -LiteralPath $generatedIndex) {
        Remove-Item -LiteralPath $generatedIndex -Force
    }

    if (Test-Path -LiteralPath $outPath) {
        Remove-Item -LiteralPath $outPath -Force
    }

    Compress-Archive -LiteralPath $stageRoot -DestinationPath $outPath -CompressionLevel Optimal
    Write-Host "Packaged: $outPath"
}
finally {
    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}
