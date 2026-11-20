param([string]$PackagePath = 'dist/staging/YTFrame-win64')
$ErrorActionPreference = 'Stop'
function Samples($Path) {
    $Values = @()
    foreach ($Index in 0..5) {
        $Timer = [Diagnostics.Stopwatch]::StartNew()
        & $Path --ignore-config --version | Out-Null
        if ($LASTEXITCODE -ne 0) { throw "Resolver benchmark failed: $Path" }
        $Timer.Stop()
        $Values += [Math]::Round($Timer.Elapsed.TotalMilliseconds, 2)
    }
    return $Values
}
function Median($Values) {
    $Sorted = @($Values | Sort-Object)
    return $Sorted[[int][Math]::Floor($Sorted.Count / 2)]
}
$OneFile = @(Samples (Join-Path (Get-Location) 'bundle/yt-dlp.exe'))
$Unpacked = @(Samples (Join-Path (Get-Location) "$PackagePath/resolver/yt-dlp.exe"))
$Result = [ordered]@{
    platform = [Environment]::OSVersion.VersionString
    resolver_version = '2026.10.26'
    operation = '--ignore-config --version (process startup only, no network)'
    onefile_ms = $OneFile
    unpacked_ms = $Unpacked
    onefile_warm_median_ms = Median ($OneFile[1..5])
    unpacked_warm_median_ms = Median ($Unpacked[1..5])
}
$Result | ConvertTo-Json -Depth 4 | Set-Content "$PackagePath/RESOLVER_BENCHMARK.json"
& go test -c -o "$PackagePath/YTFrame-performance.exe" ./cmd/ytframe
if ($LASTEXITCODE -ne 0) { throw 'Could not build playback benchmark' }
try {
    $env:YTFRAME_BENCHMARK = '1'
    $env:YTFRAME_BENCHMARK_OUTPUT = (Join-Path (Get-Location) "$PackagePath/PLAYBACK_BENCHMARK.json")
    & "$PackagePath/YTFrame-performance.exe" '-test.run=^TestPlaybackStartupBenchmark$' '-test.v' '-test.timeout=2m'
    if ($LASTEXITCODE -ne 0) { throw 'Playback benchmark failed' }
} finally {
    Remove-Item "$PackagePath/YTFrame-performance.exe" -ErrorAction SilentlyContinue
    Remove-Item Env:YTFRAME_BENCHMARK,Env:YTFRAME_BENCHMARK_OUTPUT -ErrorAction SilentlyContinue
}
Write-Host ($Result | ConvertTo-Json -Depth 4)
