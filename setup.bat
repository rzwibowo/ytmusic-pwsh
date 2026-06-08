@echo off
title Setup YouTube Music Player
setlocal enabledelayedexpansion

set SCRIPT_DIR=%~dp0

powershell -NoProfile -ExecutionPolicy Bypass -Command "
$ErrorActionPreference = 'Stop';
$root = Split-Path -LiteralPath '%SCRIPT_DIR%' -Parent;
function Find-Exe { param([string[]]$names); foreach ($name in $names) { $cmd = Get-Command $name -ErrorAction SilentlyContinue; if ($cmd) { return $cmd.Path } } return $null }
$nodePath = Find-Exe -names @('node.exe');
$vlcPath = Find-Exe -names @('vlc.exe');
if (-not $nodePath) {
    Write-Host 'Node.js tidak ditemukan di PATH.';
    $inputNode = Read-Host 'Masukkan path lengkap ke node.exe jika sudah terpasang, atau tekan Enter untuk gunakan node dari PATH nanti';
    if ($inputNode) { $nodePath = $inputNode }
}
if (-not $vlcPath) {
    Write-Host 'VLC tidak ditemukan di PATH.';
    $inputVlc = Read-Host 'Masukkan path lengkap ke vlc.exe';
    if ($inputVlc) { $vlcPath = $inputVlc }
}
$ytDlpPath = Join-Path $root 'yt-dlp.exe';
if (-not (Test-Path $ytDlpPath)) {
    Write-Host 'Mengunduh yt-dlp.exe...';
    Invoke-WebRequest -Uri 'https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp.exe' -OutFile $ytDlpPath;
    Write-Host 'yt-dlp.exe berhasil diunduh ke:' $ytDlpPath;
} else {
    Write-Host 'yt-dlp.exe sudah ada di:' $ytDlpPath;
}
$file = Join-Path $root 'playerv2.ps1';
if (-not (Test-Path $file)) { throw 'File playerv2.ps1 tidak ditemukan.' }
$content = Get-Content -Path $file -Raw;
if ($vlcPath) { $content = [Regex]::Replace($content, 'VLCPath = ".*?"', "VLCPath = \"$vlcPath\"") }
$content = [Regex]::Replace($content, 'YtDlp = ".*?"', "YtDlp = \"$ytDlpPath\"");
if ($nodePath) { $content = [Regex]::Replace($content, 'JsRuntime = ".*?"', "JsRuntime = \"$nodePath\"") } else { $content = [Regex]::Replace($content, 'JsRuntime = ".*?"', 'JsRuntime = "node"') }
Set-Content -Path $file -Value $content -Encoding UTF8;
Write-Host ''; Write-Host 'Setup selesai.';
Write-Host 'Pastikan Node.js dan VLC sudah terpasang jika belum.';
Write-Host 'Jalankan runplayer.bat untuk memulai aplikasi.';
" 
pause
