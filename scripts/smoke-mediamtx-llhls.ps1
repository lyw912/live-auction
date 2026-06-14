param(
  [string]$ComposeFile = "infra/docker-compose.yml",
  [string]$HlsURL = "http://127.0.0.1:8888/auction-live/index.m3u8?cookieCheck=1"
)

$ErrorActionPreference = "Stop"

function Test-HlsReady {
  param([string]$URL)
  try {
    $content = & curl.exe -fsS $URL
    return ($LASTEXITCODE -eq 0 -and (($content -join "`n").Contains("#EXTM3U")))
  } catch {
    return $false
  }
}

if (-not (Test-HlsReady -URL $HlsURL)) {
  docker compose -f $ComposeFile up -d mediamtx
}

$deadline = (Get-Date).AddSeconds(60)
$lastError = $null
do {
  try {
    if (Test-HlsReady -URL $HlsURL) {
      Write-Host "MediaMTX LL-HLS ready: $HlsURL"
      Write-Host "Backend descriptor env:"
      Write-Host "  LIVE_DEMO_MEDIA_PROTOCOL=ll-hls"
      Write-Host "  LIVE_DEMO_MEDIA_URL=$HlsURL"
      Write-Host "  LIVE_DEMO_MIME_TYPE=application/vnd.apple.mpegurl"
      Write-Host "  LIVE_DEMO_IS_LIVE=true"
      Write-Host "  LIVE_DEMO_LATENCY_MS=3000"
      Write-Host "  LIVE_MEDIA_FALLBACK_MP4_URL=/demo/jade-live-loop.mp4"
      exit 0
    }
    $lastError = "Unexpected response: $($response.StatusCode)"
  } catch {
    $lastError = $_.Exception.Message
  }
  Start-Sleep -Seconds 2
} while ((Get-Date) -lt $deadline)

throw "MediaMTX LL-HLS smoke failed for $HlsURL. Last error: $lastError"
