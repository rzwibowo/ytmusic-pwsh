@echo off
title Setup YouTube Music Player
setlocal
pushd "%~dp0"

where go >nul 2>nul
if errorlevel 1 (
    echo Go tidak ditemukan di PATH. Instal Go terlebih dahulu.
    pause
    exit /b 1
)

powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$ErrorActionPreference='Stop';" ^
  "function Find-Exe([string[]]$Names) { foreach($name in $Names) { $cmd=Get-Command $name -ErrorAction SilentlyContinue; if($cmd){ return $cmd.Path } }; return '' };" ^
  "$root=(Get-Location).Path;" ^
  "$node=Find-Exe @('node.exe');" ^
  "$vlc=Find-Exe @('vlc.exe');" ^
  "if(-not $vlc) { $common=@($env:ProgramFiles + '\VideoLAN\VLC\vlc.exe', ${env:ProgramFiles(x86)} + '\VideoLAN\VLC\vlc.exe'); $vlc=$common | Where-Object { Test-Path $_ } | Select-Object -First 1 };" ^
  "if(-not $vlc) { $vlc=Read-Host 'Masukkan path lengkap ke vlc.exe' };" ^
  "$yt=Join-Path $root 'yt-dlp.exe';" ^
  "if(-not (Test-Path $yt)) { Write-Host 'Mengunduh yt-dlp.exe...'; Invoke-WebRequest 'https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp.exe' -OutFile $yt };" ^
  "$cfg=[ordered]@{cacheLimitMB=500;vlcPath=$vlc;ytDlp=$yt;jsRuntime=($(if($node){$node}else{'node'}));httpPort=9494;httpPassword='ytmusic';dataDir='.\data';cacheDir='.\cache';playlistLibraryDir='.\data\playlists';thumbnailWidth=32;lyricsApi='https://lrclib.net/api/search'};" ^
  "$json=$cfg | ConvertTo-Json;" ^
  "[IO.File]::WriteAllText((Join-Path $root 'config.json'), $json, [Text.UTF8Encoding]::new($false))"

if errorlevel 1 (
    echo Setup dependensi gagal.
    pause
    exit /b 1
)

set GOCACHE=%CD%\.gocache
go build -o ytplayer.exe .
if errorlevel 1 (
    echo Build Go gagal.
    pause
    exit /b 1
)

echo.
echo Setup selesai. Jalankan runplayer.bat.
popd
pause
