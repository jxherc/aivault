# installs aivault on windows:
#   irm https://raw.githubusercontent.com/jxherc/aivault/main/install.ps1 | iex
# env: AIVAULT_VERSION (default latest), AIVAULT_DIR (default %LOCALAPPDATA%\Programs\aivault),
# AIVAULT_NO_INIT=1 to skip the setup question, AIVAULT_BASE_URL to download
# from somewhere other than github releases
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue' # the progress bar makes downloads crawl

$repo = 'jxherc/aivault'
$dir = if ($env:AIVAULT_DIR) { $env:AIVAULT_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\aivault' }
$ver = if ($env:AIVAULT_VERSION) { $env:AIVAULT_VERSION } else { 'latest' }

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
  'AMD64' { 'x64' }
  'ARM64' { 'arm64' }
  default { throw "no build for $env:PROCESSOR_ARCHITECTURE. try: go install github.com/$repo/cmd/aivault@latest" }
}
$base = if ($env:AIVAULT_BASE_URL) { $env:AIVAULT_BASE_URL } elseif ($ver -eq 'latest') { "https://github.com/$repo/releases/latest/download" } else { "https://github.com/$repo/releases/download/v$($ver.TrimStart('v'))" }
$file = "aivault-windows-$arch.zip"
$tmp = Join-Path ([IO.Path]::GetTempPath()) ("aivault-" + [guid]::NewGuid())
New-Item -ItemType Directory $tmp | Out-Null

try {
  Write-Host "downloading $file"
  try {
    Invoke-WebRequest "$base/$file" -OutFile "$tmp\$file" -UseBasicParsing
    Invoke-WebRequest "$base/checksums.txt" -OutFile "$tmp\checksums.txt" -UseBasicParsing
  } catch {
    throw "download failed. is there a release yet? see https://github.com/$repo/releases"
  }
  $want = (Get-Content "$tmp\checksums.txt" | Where-Object { $_ -match " $([regex]::Escape($file))$" }) -split ' ' | Select-Object -First 1
  $got = (Get-FileHash "$tmp\$file" -Algorithm SHA256).Hash.ToLower()
  if (-not $want -or $want -ne $got) { throw 'checksum mismatch, not installing' }

  Expand-Archive "$tmp\$file" -DestinationPath "$tmp\x" -Force
  New-Item -ItemType Directory $dir -Force | Out-Null
  $exe = Join-Path $dir 'aivault.exe'
  # a running mcp server locks the exe, but windows lets it be renamed
  if (Test-Path $exe) { Move-Item $exe "$exe.old" -Force }
  Copy-Item "$tmp\x\aivault.exe" $exe -Force
  Write-Host "installed $(& $exe version) to $exe"

  $path = [Environment]::GetEnvironmentVariable('Path', 'User')
  if (($path -split ';') -notcontains $dir) {
    [Environment]::SetEnvironmentVariable('Path', ($path.TrimEnd(';') + ";$dir").TrimStart(';'), 'User')
    $env:Path += ";$dir"
    Write-Host "added $dir to your PATH. open a new terminal for it to stick."
  }
} finally {
  Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
}

if (Test-Path (Join-Path $HOME '.aivault\config.json')) {
  Write-Host "`nalready set up. ``aivault doctor`` checks everything still works."
} elseif ($env:AIVAULT_NO_INIT) {
  Write-Host 'next: aivault init'
} else {
  $ans = Read-Host "`nset up your vault now? [Y/n]"
  if ($ans -match '^[nN]') { Write-Host 'later: aivault init' } else { & $exe init }
}
