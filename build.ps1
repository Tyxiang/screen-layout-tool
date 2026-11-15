param(
    [switch]$BumpPatch
)

$ErrorActionPreference = 'Stop'
Set-Location -LiteralPath $PSScriptRoot
Get-Location

$versionPath = Join-Path $PSScriptRoot 'VERSION'
$version = (Get-Content -LiteralPath $versionPath -Encoding UTF8 -Raw).Trim()
if ($version -notmatch '^\d+\.\d+\.\d+$') { throw 'VERSION 格式必须为 主版本.次版本.修订版本。' }
if ($BumpPatch) {
    $parts = $version.Split('.')
    $version = '{0}.{1}.{2}' -f $parts[0], $parts[1], ([int]$parts[2] + 1)
}

$env:CGO_ENABLED = '0'
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
$env:GOMODCACHE = Join-Path $PSScriptRoot '.cache\modules'
$env:GOCACHE = Join-Path $PSScriptRoot '.cache\build'

go test ./...
if ($LASTEXITCODE -ne 0) { throw '测试失败，已停止构建。' }
go vet ./...
if ($LASTEXITCODE -ne 0) { throw '静态检查失败，已停止构建。' }

$releaseDirectory = Join-Path $PSScriptRoot ('bin\screen-layout-tool-' + $version)
New-Item -ItemType Directory -Path $releaseDirectory -Force | Out-Null
go build -trimpath -ldflags "-H=windowsgui -X main.version=$version" -o (Join-Path $releaseDirectory 'screen-layout-tool.exe') .
if ($LASTEXITCODE -ne 0) { throw '构建失败。' }
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'layouts') -Destination $releaseDirectory -Recurse -Force
if ($BumpPatch) { Set-Content -LiteralPath $versionPath -Value $version -Encoding UTF8 }
Write-Output "构建完成：$releaseDirectory"
