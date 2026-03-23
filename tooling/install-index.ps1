param(
    [string]$ApiUrl
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Net.Http

$defaultApiUrl = 'https://rbf-api.yourdomain.com/index'
$resolvedApiUrl = if ($ApiUrl) {
    $ApiUrl
} elseif ($env:RBF_API_URL) {
    $env:RBF_API_URL
} else {
    $defaultApiUrl
}

$datPath = Join-Path $env:APPDATA 'Factorio\blueprint-storage-2.dat'
if (-not (Test-Path -LiteralPath $datPath)) {
    throw "Missing blueprint storage file: $datPath"
}

$modsDir = Join-Path $env:APPDATA 'Factorio\mods'
if (-not (Test-Path -LiteralPath $modsDir)) {
    throw "Missing Factorio mods directory: $modsDir"
}

function Get-ModArchiveVersion {
    param([System.IO.FileInfo]$File)

    if ($File.BaseName -match '^recursive-blueprint-finder_(?<version>\d+\.\d+\.\d+)$') {
        try {
            return [version]$Matches.version
        } catch {
            return [version]'0.0.0'
        }
    }

    return [version]'0.0.0'
}

$zipInfo = Get-ChildItem -LiteralPath $modsDir -Filter 'recursive-blueprint-finder_*.zip' -File |
    Sort-Object -Property `
        @{ Expression = { Get-ModArchiveVersion $_ }; Descending = $true }, `
        @{ Expression = { $_.LastWriteTimeUtc }; Descending = $true } |
    Select-Object -First 1

if (-not $zipInfo) {
    throw "Could not find recursive-blueprint-finder_*.zip in $modsDir"
}

$client = [System.Net.Http.HttpClient]::new()
$form = [System.Net.Http.MultipartFormDataContent]::new()
$fileStream = [System.IO.File]::OpenRead($datPath)
$fileContent = [System.Net.Http.StreamContent]::new($fileStream)

$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ('rbf-install-' + [guid]::NewGuid().ToString('N'))
$expandRoot = Join-Path $tempRoot 'expanded'
$rebuiltZip = Join-Path $tempRoot $zipInfo.Name

try {
    $fileContent.Headers.ContentType = [System.Net.Http.Headers.MediaTypeHeaderValue]::Parse('application/octet-stream')
    $form.Add($fileContent, 'blueprint_storage', [System.IO.Path]::GetFileName($datPath))

    $response = $client.PostAsync($resolvedApiUrl, $form).GetAwaiter().GetResult()
    $indexLua = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()

    if (-not $response.IsSuccessStatusCode) {
        throw "API request failed: $([int]$response.StatusCode) $($response.ReasonPhrase)`n$indexLua"
    }

    New-Item -ItemType Directory -Path $expandRoot -Force | Out-Null
    Expand-Archive -LiteralPath $zipInfo.FullName -DestinationPath $expandRoot -Force

    $modRoot = Get-ChildItem -LiteralPath $expandRoot -Directory | Select-Object -First 1
    if (-not $modRoot) {
        throw 'Expanded mod archive did not contain a top-level mod directory.'
    }

    $generatedDir = Join-Path $modRoot.FullName 'generated'
    New-Item -ItemType Directory -Path $generatedDir -Force | Out-Null

    $indexPath = Join-Path $generatedDir 'index.lua'
    Set-Content -LiteralPath $indexPath -Value $indexLua -Encoding utf8

    if (Test-Path -LiteralPath $rebuiltZip) {
        Remove-Item -LiteralPath $rebuiltZip -Force
    }

    Compress-Archive -LiteralPath $modRoot.FullName -DestinationPath $rebuiltZip -CompressionLevel Optimal

    Move-Item -LiteralPath $rebuiltZip -Destination $zipInfo.FullName -Force
    Write-Host "Injected prebuilt index into $($zipInfo.FullName)"
}
finally {
    $fileContent.Dispose()
    $fileStream.Dispose()
    $form.Dispose()
    $client.Dispose()

    if (Test-Path -LiteralPath $tempRoot) {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
}
