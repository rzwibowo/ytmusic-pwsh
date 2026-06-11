@echo off
title YouTube Music Player

if not exist "%~dp0ytplayer.exe" (
    echo ytplayer.exe belum ada. Jalankan setup.bat terlebih dahulu.
    pause
    exit /b 1
)

pushd "%~dp0"
"%~dp0ytplayer.exe"
popd

pause
