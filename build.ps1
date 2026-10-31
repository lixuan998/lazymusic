[CmdletBinding()]
param(
 [ValidateSet('build','test','vet','install','uninstall')][string]$Task='build',
 [string]$Prefix=(Join-Path $env:LOCALAPPDATA 'Programs\lazymusic'),
 [switch]$InstallNvim,
 [string]$NvimConfigDir=(Join-Path $env:LOCALAPPDATA 'nvim')
)
$ErrorActionPreference='Stop'
$root=$PSScriptRoot
$binary=Join-Path $root 'bin\lazymusic.exe'
$installed=Join-Path $Prefix 'bin\lazymusic.exe'
if ($Task -eq 'uninstall') {
 if (Test-Path -LiteralPath $installed) { Remove-Item -LiteralPath $installed }
 Write-Host 'Removed executable; configuration and sessions are preserved.'
 return
}
foreach ($tool in @('go','gcc','g++')) {
 if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) {
  throw "Missing $tool. Install Go 1.25+ and 64-bit MinGW-w64 GCC, and add their bin directories to PATH. See README.md."
 }
}
function Invoke-Go {
 & go @args
 if ($LASTEXITCODE -ne 0) { throw "go $($args -join ' ') failed ($LASTEXITCODE)" }
}
$previousCGO=$env:CGO_ENABLED
$previousCC=$env:CC
$previousCXX=$env:CXX
$stage=$null
Push-Location $root
try {
 $env:CGO_ENABLED='1'
 if (-not $env:CC) { $env:CC='gcc' }
 if (-not $env:CXX) { $env:CXX='g++' }
 # Go cannot lock go.mod on WSL UNC filesystems; build on NTFS and copy back.
 if ($root.StartsWith('\\')) {
  $stage=Join-Path ([IO.Path]::GetTempPath()) ('lazymusic-build-'+[guid]::NewGuid().ToString('N'))
  New-Item -ItemType Directory -Path $stage | Out-Null
  Get-ChildItem -LiteralPath $root -Force | Where-Object { $_.Name -notin @('.git','bin') } | ForEach-Object {
   Copy-Item -LiteralPath $_.FullName -Destination $stage -Recurse -Force
  }
  Set-Location $stage
 }
 switch ($Task) {
  'test' { Invoke-Go test -count=1 -timeout=120s ./... }
  'vet' { Invoke-Go vet ./... }
  default {
   New-Item -ItemType Directory -Force -Path bin | Out-Null
   Invoke-Go build -buildvcs=false -trimpath -o bin/lazymusic.exe ./cmd/lazymusic
   if ($stage) {
    New-Item -ItemType Directory -Force -Path (Split-Path $binary) | Out-Null
    Copy-Item -LiteralPath (Join-Path $stage 'bin\lazymusic.exe') -Destination $binary -Force
   }
   Write-Host "Built $binary"
  }
 }
 if ($Task -eq 'install') {
  New-Item -ItemType Directory -Force -Path (Split-Path $installed) | Out-Null
  Copy-Item -LiteralPath $binary -Destination $installed -Force
  if ($InstallNvim) {
   $plugin=Join-Path $Prefix 'share\lazymusic\nvim'
   $lua=Join-Path $plugin 'lua\lazymusic'
   $help=Join-Path $plugin 'doc'
   New-Item -ItemType Directory -Force -Path $lua,$help | Out-Null
   Copy-Item -LiteralPath (Join-Path $root 'lua\lazymusic\init.lua'),(Join-Path $root 'lua\lazymusic\graphics.lua') -Destination $lua -Force
   Copy-Item -LiteralPath (Join-Path $root 'docs\lazymusic.txt') -Destination $help -Force
   & $installed setup-nvim --config-dir $NvimConfigDir --plugin-dir $plugin --binary $installed --help-dir $help
   if ($LASTEXITCODE -ne 0) { throw 'Neovim setup failed' }
  }
  Write-Host "Installed $installed. Add $(Split-Path $installed) to your user PATH."
 }
}
finally {
 Pop-Location
 $env:CGO_ENABLED=$previousCGO
 $env:CC=$previousCC
 $env:CXX=$previousCXX
 if ($stage) {
  $resolvedStage=[IO.Path]::GetFullPath($stage)
  $tempRoot=[IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\')+'\'
  if (-not $resolvedStage.StartsWith($tempRoot,[StringComparison]::OrdinalIgnoreCase) -or
      (Split-Path $resolvedStage -Leaf) -notmatch '^lazymusic-build-[0-9a-f]{32}$') { throw 'Unsafe staging path' }
  Remove-Item -LiteralPath $resolvedStage -Recurse -Force
 }
}
