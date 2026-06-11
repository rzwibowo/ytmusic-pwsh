# YouTube Music Player

Pemutar musik YouTube berbasis terminal. Implementasi utama kini menggunakan Go
dan menghasilkan satu executable Windows.

## Fitur

- Pencarian, playlist, profile playlist, dan rekomendasi YouTube
- Playback headless melalui VLC HTTP interface
- Queue, shuffle, auto-next, preload, dan cache audio
- Local playlist library dengan data yang kompatibel dengan versi PowerShell
- Lirik LRCLIB dan thumbnail true-color di terminal
- Hotkey Space, panah, F1, dan F8

## Persyaratan

- Go 1.25+ untuk build
- VLC media player
- Node.js sebagai JavaScript runtime `yt-dlp`
- Koneksi internet

## Setup

Jalankan:

```bat
setup.bat
```

Setup akan mendeteksi VLC dan Node.js, mengunduh `yt-dlp.exe`, membuat
`config.json`, lalu membangun `ytplayer.exe`.

## Menjalankan

```bat
runplayer.bat
```

Atau:

```powershell
.\ytplayer.exe
```

Port VLC dapat dioverride:

```powershell
.\ytplayer.exe -http-port 9595
```

Konfigurasi lokal disimpan di `config.json`. File tersebut sengaja tidak masuk
Git karena berisi path executable spesifik komputer.

## Build Manual

```powershell
$env:GOCACHE = "$PWD\.gocache"
go test ./...
go build -o ytplayer.exe .
```

Versi PowerShell lama tetap tersedia di `playerv2.ps1` sebagai referensi.
