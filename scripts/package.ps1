# Build the portable Windows package from pinned upstream components.
$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..')
function Fetch-Verified($Url, $Path, $SHA256) {
    if (!(Test-Path $Path)) { Invoke-WebRequest $Url -OutFile $Path }
    if ((Get-FileHash $Path -Algorithm SHA256).Hash -ne $SHA256) {
        throw "Checksum mismatch: $Path"
    }
}
New-Item -ItemType Directory -Force bundle | Out-Null
Fetch-Verified 'https://github.com/zhongfly/mpv-winbuild/releases/download/2026-11-18-f92afa75cd/mpv-x86_64-20261118-git-f92afa75cd.7z' 'bundle/mpv.7z' '6c99e301d8fd858deeda6ebe3328a74c6a9d76ac69ae63cbcc80ea634d00cb38'
Fetch-Verified 'https://github.com/yt-dlp/yt-dlp/releases/download/2026.10.26/yt-dlp.exe' 'bundle/yt-dlp.exe' 'f5389d3698ecde4ef06f4e6b1ce0294fe9ac1396228c8990b8d3416d6d177a7d'
& 7z e bundle/mpv.7z '-obundle' mpv.exe -y | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'mpv extraction failed' }
# Stage separately so rebuilding never erases a portable install's settings/profile.
$Stage = 'dist/staging/YTFrame-win64'
if (Test-Path $Stage) { Remove-Item -Recurse -Force $Stage }
New-Item -ItemType Directory -Force $Stage | Out-Null
$env:CGO_ENABLED = '0'
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
& go build -buildvcs=false -trimpath -ldflags '-H windowsgui -s -w' -o "$Stage/YTFrame.exe" ./cmd/ytframe
if ($LASTEXITCODE -ne 0) { throw 'Go build failed' }
Copy-Item bundle/mpv.exe,bundle/yt-dlp.exe,README.txt,LICENSES.txt,SOURCES.txt,LICENSE,LICENSE.GPL-2.0 $Stage
Copy-Item docs/CODE_SIGNING.md "$Stage/CODE_SIGNING.txt"
$Zip = 'dist/YTFrame-win64.zip'
if (Test-Path $Zip) { Remove-Item $Zip }
Push-Location dist/staging
try {
    & 7z a -tzip ../YTFrame-win64.zip YTFrame-win64 | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Packaging failed' }
} finally { Pop-Location }
if ($LASTEXITCODE -ne 0) { throw 'Packaging failed' }
$Hash = (Get-FileHash $Zip -Algorithm SHA256).Hash.ToLowerInvariant()
[IO.File]::WriteAllText((Join-Path (Get-Location) 'dist/YTFrame-win64.sha256'), "$Hash  YTFrame-win64.zip`n")
Write-Host "Built $Zip (unsigned; see docs/CODE_SIGNING.md)"
