Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$sourceDir = if ($env:SOURCE_DIR) { $env:SOURCE_DIR } else { "source" }
$outputDir = if ($env:OUTPUT_DIR) { $env:OUTPUT_DIR } else { "dist/windows" }
$version = $env:VERSION

if ([string]::IsNullOrWhiteSpace($version)) {
  throw "VERSION is required"
}

$wailsConfig = Get-Content (Join-Path $sourceDir "wails.json") -Raw | ConvertFrom-Json
$productName = if ($env:PRODUCT_NAME) { $env:PRODUCT_NAME } elseif ($wailsConfig.outputfilename) { $wailsConfig.outputfilename } else { $wailsConfig.name }
$platform = if ($env:WINDOWS_WAILS_PLATFORM) { $env:WINDOWS_WAILS_PLATFORM } else { "windows/amd64" }
$timestampUrl = if ($env:WINDOWS_TIMESTAMP_URL) { $env:WINDOWS_TIMESTAMP_URL } else { "http://timestamp.digicert.com" }

New-Item -ItemType Directory -Path $outputDir -Force | Out-Null

Push-Location $sourceDir
wails build -clean -platform $platform
Pop-Location

$exePath = Join-Path $sourceDir "build/bin/$productName.exe"
if (-not (Test-Path $exePath)) {
  throw "Expected executable not found at $exePath"
}

$signtool = (Get-Command signtool.exe).Source
& $signtool sign `
  /fd SHA256 `
  /f $env:WINDOWS_CERTIFICATE_PATH `
  /p $env:WINDOWS_CERTIFICATE_PASSWORD `
  /tr $timestampUrl `
  /td SHA256 `
  /a `
  $exePath

$zipPath = Join-Path $outputDir "$productName-$version-windows.zip"
Compress-Archive -Path $exePath -DestinationPath $zipPath -Force
