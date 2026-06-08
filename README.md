# YouTube Music Player

Aplikasi ini membutuhkan beberapa dependensi sebelum dapat dijalankan.

<img width="995" height="538" alt="image" src="https://github.com/user-attachments/assets/df460263-3280-4c08-93e6-458c4251ba13" />


## Persyaratan

- `PowerShell 7+` (disarankan `pwsh`)
- `Node.js` sebagai runtime
- `VLC` media player
- `yt-dlp`

> Pastikan menggunakan PowerShell 7 atau lebih baru, karena `playerv2.ps1` membutuhkan fitur PowerShell 7.

## Cara pakai

1. Sesuaikan lokasi file jika tidak terdeteksi otomatis.
2. Jalankan `setup.bat` dari folder proyek untuk mengunduh `yt-dlp` dan memperbarui konfigurasi.
3. Jalankan `runplayer.bat` untuk memulai aplikasi.

## Konfigurasi

File konfigurasi utama ada di `playerv2.ps1`.
Sesuaikan nilai berikut jika diperlukan:

- `VLCPath` : lokasi lengkap `vlc.exe`
- `YtDlp` : lokasi lengkap `yt-dlp.exe`
- `ThumbnailWidth` : lebar thumbnail ANSI berwarna di terminal
- `JsRuntime` : `node` atau path lengkap ke `node.exe`

Contoh konfigurasi di `playerv2.ps1`:

```powershell
    VLCPath = "C:\Path\To\VLC\vlc.exe"
    YtDlp = "C:\Path\To\yt-dlp.exe"
    ThumbnailWidth = 32
    JsRuntime = "node"
```

## Setup awal

Jalankan:

```bat
setup.bat
```

Skrip akan:

- mendeteksi `Node.js` dan `VLC`
- mengunduh `yt-dlp.exe` jika belum ada
- memperbarui nilai `VLCPath`, `YtDlp`, dan `JsRuntime` di `playerv2.ps1`

> Jika `Node.js` atau `VLC` belum terpasang, instal manual dari situs resmi, lalu jalankan kembali `setup.bat`.

## Menjalankan aplikasi

Setelah setup selesai, jalankan:

```bat
runplayer.bat
```

atau langsung:

```powershell
pwsh -ExecutionPolicy Bypass -NoProfile -File ".\playerv2.ps1"
```
