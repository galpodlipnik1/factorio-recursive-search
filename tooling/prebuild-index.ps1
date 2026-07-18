param(
    [string]$ApiUrl = 'http://localhost:8080/index',
    [string]$BlueprintStoragePath,
    [string]$OutputPath
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Net.Http

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$resolvedStoragePath = if ($BlueprintStoragePath) {
    $BlueprintStoragePath
} else {
    Join-Path $env:APPDATA 'Factorio\blueprint-storage-2.dat'
}
$resolvedOutputPath = if ($OutputPath) {
    $OutputPath
} else {
    Join-Path $repoRoot 'generated\index.lua'
}

if (-not (Test-Path -LiteralPath $resolvedStoragePath -PathType Leaf)) {
    throw "Missing blueprint storage file: $resolvedStoragePath"
}

$outputDirectory = Split-Path -Parent $resolvedOutputPath
if ([string]::IsNullOrWhiteSpace($outputDirectory)) {
    throw "OutputPath must include a directory: $resolvedOutputPath"
}

$client = [System.Net.Http.HttpClient]::new()
$form = [System.Net.Http.MultipartFormDataContent]::new()
$fileStream = [System.IO.File]::OpenRead($resolvedStoragePath)
$fileContent = [System.Net.Http.StreamContent]::new($fileStream)
$temporaryPath = Join-Path $outputDirectory ('index-' + [guid]::NewGuid().ToString('N') + '.tmp')

try {
    $fileContent.Headers.ContentType = [System.Net.Http.Headers.MediaTypeHeaderValue]::Parse('application/octet-stream')
    $form.Add($fileContent, 'blueprint_storage', [System.IO.Path]::GetFileName($resolvedStoragePath))

    $response = $client.PostAsync($ApiUrl, $form).GetAwaiter().GetResult()
    $indexLua = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()

    if (-not $response.IsSuccessStatusCode) {
        throw "API request failed: $([int]$response.StatusCode) $($response.ReasonPhrase)`n$indexLua"
    }
    if ($indexLua -notmatch '(?m)^return\s*\{' -or $indexLua -notmatch 'schema_version\s*=\s*2') {
        throw 'API returned a successful response that is not a schema-v2 Lua index.'
    }

    $entryCountHeader = $response.Headers.GetValues('X-Rbf-Entry-Count') | Select-Object -First 1
    $entryCount = 0
    if (-not [int]::TryParse($entryCountHeader, [ref]$entryCount) -or $entryCount -lt 0) {
        throw "API returned an invalid X-Rbf-Entry-Count header: $entryCountHeader"
    }

    New-Item -ItemType Directory -Path $outputDirectory -Force | Out-Null
    $utf8NoBom = [System.Text.UTF8Encoding]::new($false)
    [System.IO.File]::WriteAllText($temporaryPath, $indexLua, $utf8NoBom)
    Move-Item -LiteralPath $temporaryPath -Destination $resolvedOutputPath -Force

    Write-Host "Prebuilt $entryCount entries into $resolvedOutputPath"
}
finally {
    $fileContent.Dispose()
    $fileStream.Dispose()
    $form.Dispose()
    $client.Dispose()

    if (Test-Path -LiteralPath $temporaryPath) {
        Remove-Item -LiteralPath $temporaryPath -Force
    }
}
