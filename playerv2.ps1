#Requires -Version 7.0

param(
    [ValidateRange(1, 65535)]
    [int]$HttpPort = 9494
)

Set-StrictMode -Version Latest

$ErrorActionPreference = "Stop"

# ============================================================
# CONFIG
# ============================================================

$Config = @{
    CacheLimitMB = 500

    VLCPath = "F:\xxx\VideoLAN\VLC\vlc.exe"

    YtDlp = "F:\xxx\ytdlp\yt-dlp.exe"

    JsRuntime = "node"

    HttpPort = $HttpPort

    HttpPassword = "ytmusic"

    DataDir = ".\data"

    CacheDir = ".\cache"

    PlaylistLibraryDir = ".\data\playlists"

    ThumbnailWidth = 32

    LyricsApi = "https://lrclib.net/api/search"
}

# ============================================================
# DIRECTORIES
# ============================================================

$null = New-Item $Config.DataDir -ItemType Directory -Force
$null = New-Item $Config.CacheDir -ItemType Directory -Force
$null = New-Item $Config.PlaylistLibraryDir -ItemType Directory -Force

# ============================================================
# GLOBAL STATE
# ============================================================

$script:CurrentPlaylist = @()

$script:Queue = New-Object System.Collections.ArrayList

$script:CurrentIndex = -1

$script:Shuffle = $false

$script:AutoRecommend = $true

$script:CurrentSong = $null

$script:VLCProcess = $null

$script:VLCJobInitialized = $false

$script:AutoAdvanceArmed = $false

$script:ProfilePlaylistResults = @()

$script:PlaylistLibrary = @()

# ============================================================
# HTTP AUTH
# ============================================================

function Get-VLCAuthHeader {

    $pair = ":$($Config.HttpPassword)"

    $bytes =
        [System.Text.Encoding]::ASCII.GetBytes($pair)

    $token =
        [Convert]::ToBase64String($bytes)

    @{
        Authorization = "Basic $token"
    }
}

# ============================================================
# VLC
# ============================================================

function Initialize-VLCJob {

    if ($script:VLCJobInitialized) {
        return
    }

    if (!("VLCProcessJob" -as [type])) {
        Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;

public static class VLCProcessJob
{
    private const uint JobObjectExtendedLimitInformation = 9;
    private const uint JobObjectLimitKillOnJobClose = 0x00002000;
    private static IntPtr jobHandle = IntPtr.Zero;

    [StructLayout(LayoutKind.Sequential)]
    private struct IO_COUNTERS
    {
        public ulong ReadOperationCount;
        public ulong WriteOperationCount;
        public ulong OtherOperationCount;
        public ulong ReadTransferCount;
        public ulong WriteTransferCount;
        public ulong OtherTransferCount;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct JOBOBJECT_BASIC_LIMIT_INFORMATION
    {
        public long PerProcessUserTimeLimit;
        public long PerJobUserTimeLimit;
        public uint LimitFlags;
        public UIntPtr MinimumWorkingSetSize;
        public UIntPtr MaximumWorkingSetSize;
        public uint ActiveProcessLimit;
        public UIntPtr Affinity;
        public uint PriorityClass;
        public uint SchedulingClass;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct JOBOBJECT_EXTENDED_LIMIT_INFORMATION
    {
        public JOBOBJECT_BASIC_LIMIT_INFORMATION BasicLimitInformation;
        public IO_COUNTERS IoInfo;
        public UIntPtr ProcessMemoryLimit;
        public UIntPtr JobMemoryLimit;
        public UIntPtr PeakProcessMemoryUsed;
        public UIntPtr PeakJobMemoryUsed;
    }

    [DllImport("kernel32.dll", CharSet = CharSet.Unicode)]
    private static extern IntPtr CreateJobObject(IntPtr attributes, string name);

    [DllImport("kernel32.dll")]
    private static extern bool SetInformationJobObject(
        IntPtr job,
        uint infoClass,
        IntPtr info,
        uint length
    );

    [DllImport("kernel32.dll")]
    private static extern bool AssignProcessToJobObject(
        IntPtr job,
        IntPtr process
    );

    public static void Initialize()
    {
        if (jobHandle != IntPtr.Zero) return;

        jobHandle = CreateJobObject(IntPtr.Zero, null);
        if (jobHandle == IntPtr.Zero)
            throw new InvalidOperationException("Could not create VLC process job.");

        var info = new JOBOBJECT_EXTENDED_LIMIT_INFORMATION();
        info.BasicLimitInformation.LimitFlags = JobObjectLimitKillOnJobClose;

        int length = Marshal.SizeOf(info);
        IntPtr pointer = Marshal.AllocHGlobal(length);

        try
        {
            Marshal.StructureToPtr(info, pointer, false);
            if (!SetInformationJobObject(
                jobHandle,
                JobObjectExtendedLimitInformation,
                pointer,
                (uint)length
            ))
                throw new InvalidOperationException("Could not configure VLC process job.");
        }
        finally
        {
            Marshal.FreeHGlobal(pointer);
        }
    }

    public static bool AddProcess(IntPtr processHandle)
    {
        Initialize();
        return AssignProcessToJobObject(jobHandle, processHandle);
    }
}
'@
    }

    [VLCProcessJob]::Initialize()
    $script:VLCJobInitialized = $true
}

function Test-VLCRunning {

    try {

        Invoke-RestMethod `
            -Uri "http://127.0.0.1:$($Config.HttpPort)/requests/status.json" `
            -Headers (Get-VLCAuthHeader) `
            -TimeoutSec 2 | Out-Null

        return $true
    }
    catch {

        return $false
    }
}

function Start-VLC {

    if (Test-VLCRunning) {
        return
    }

    Write-Host ""
    Write-Host "Starting VLC headless..."

    Initialize-VLCJob

    $script:VLCProcess = Start-Process `
        -FilePath $Config.VLCPath `
        -ArgumentList @(
            "--intf","dummy",
            "--extraintf","http",
            "--http-port",$Config.HttpPort,
            "--http-password",$Config.HttpPassword,
            "--no-video",
            "--audio-visual=none"
        ) `
        -WindowStyle Hidden `
        -PassThru

    if (![VLCProcessJob]::AddProcess($script:VLCProcess.Handle)) {
        Write-Host "Warning: VLC could not be attached to terminal cleanup"
    }

    Start-Sleep 3
}

function Invoke-VLCCommand {

    param(
        [string]$Command
    )

    $url =
        "http://127.0.0.1:$($Config.HttpPort)/requests/status.json?command=$Command"

    Invoke-RestMethod `
        -Uri $url `
        -Headers (Get-VLCAuthHeader)
}

function Get-VLCStatus {

    Invoke-RestMethod `
        -Uri "http://127.0.0.1:$($Config.HttpPort)/requests/status.json" `
        -Headers (Get-VLCAuthHeader)
}

function Stop-VLC {

    if (Test-VLCRunning) {
        try {
            Invoke-VLCCommand "pl_exit" | Out-Null
        }
        catch {
            # VLC may close the HTTP connection before returning a response.
        }
    }

    if ($script:VLCProcess) {
        try {
            if (!$script:VLCProcess.WaitForExit(3000)) {
                Stop-Process -Id $script:VLCProcess.Id -Force -ErrorAction SilentlyContinue
                $script:VLCProcess.WaitForExit(2000) | Out-Null
            }
        }
        catch {
        }

        $script:VLCProcess = $null
    }
}

# ============================================================
# TERMINAL UI
# ============================================================

function Get-TuiStatusData {

    $title =
        if ($script:CurrentSong) {
            [string]$script:CurrentSong.Title
        }
        else {
            "Nothing playing"
        }

    if ($title.Length -gt 50) {
        $title = $title.Substring(0, 47) + "..."
    }

    $state = "VLC OFFLINE"
    $position = 0
    $elapsedSeconds = 0
    $totalSeconds = 0

    try {
        $status = Get-VLCStatus
        $state = ([string]$status.state).ToUpperInvariant()
        $position = [math]::Clamp(
            [math]::Round(([double]$status.position * 100)),
            0,
            100
        )
        $elapsedSeconds = [math]::Max(0, [int]$status.time)
        $totalSeconds = [math]::Max(0, [int]$status.length)
    }
    catch {
    }

    $barWidth = 20
    $filled = [math]::Round(($position / 100) * $barWidth)
    $autoRec =
        if ($script:AutoRecommend) { "AUTO REC ON" } else { "AUTO REC OFF" }

    $elapsedTime = Format-PlaybackTime $elapsedSeconds
    $totalTime = Format-PlaybackTime $totalSeconds

    return [PSCustomObject]@{
        State = $state
        Title = $title
        Position = $position
        ElapsedTime = $elapsedTime
        TotalTime = $totalTime
        FilledBar = "#" * $filled
        EmptyBar = "-" * ($barWidth - $filled)
        AutoRec = $autoRec
    }
}

function Format-PlaybackTime {

    param(
        [int]$Seconds
    )

    $time = [TimeSpan]::FromSeconds([math]::Max(0, $Seconds))

    if ($time.TotalHours -ge 1) {
        return "{0}:{1:00}:{2:00}" -f
            [math]::Floor($time.TotalHours),
            $time.Minutes,
            $time.Seconds
    }

    return "{0}:{1:00}" -f
        [math]::Floor($time.TotalMinutes),
        $time.Seconds
}

function Write-TuiStatus {

    param(
        [int]$Row
    )

    try {
        $cursorLeft = [Console]::CursorLeft
        $cursorTop = [Console]::CursorTop
        $width = [math]::Max(20, [Console]::WindowWidth - 1)
        $status = Get-TuiStatusData
        $originalColor = [Console]::ForegroundColor

        $stateColor =
            switch ($status.State) {
                "PLAYING" { [ConsoleColor]::Green; break }
                "PAUSED" { [ConsoleColor]::Yellow; break }
                "VLC OFFLINE" { [ConsoleColor]::Red; break }
                default { [ConsoleColor]::DarkGray }
            }

        $segments = @(
            [PSCustomObject]@{ Text = "[$($status.State)] "; Color = $stateColor }
            [PSCustomObject]@{ Text = "$($status.Title) | ["; Color = [ConsoleColor]::Cyan }
            [PSCustomObject]@{ Text = $status.FilledBar; Color = [ConsoleColor]::Green }
            [PSCustomObject]@{ Text = $status.EmptyBar; Color = [ConsoleColor]::DarkGray }
            [PSCustomObject]@{ Text = "] $($status.ElapsedTime) / $($status.TotalTime) | $($status.AutoRec) | Up/Down List | F1 Help | F8 Toggle"; Color = [ConsoleColor]::Cyan }
        )

        [Console]::SetCursorPosition(0, $Row)
        $written = 0

        foreach ($segment in $segments) {
            if ($written -ge $width) {
                break
            }

            $text = [string]$segment.Text
            $available = $width - $written

            if ($text.Length -gt $available) {
                $text = $text.Substring(0, $available)
            }

            [Console]::ForegroundColor = $segment.Color
            [Console]::Write($text)
            $written += $text.Length
        }

        if ($written -lt $width) {
            [Console]::Write(" " * ($width - $written))
        }

        [Console]::ForegroundColor = $originalColor
        [Console]::SetCursorPosition($cursorLeft, $cursorTop)
    }
    catch {
        try {
            [Console]::ResetColor()
        }
        catch {
        }

        # Some redirected terminals do not support cursor positioning.
    }
}

# ============================================================
# CACHE
# ============================================================

function Get-CacheSize {

    $files =
        Get-ChildItem `
            $Config.CacheDir `
            -File `
            -ErrorAction SilentlyContinue

    if (!$files) {
        return 0
    }

    return ($files | Measure-Object Length -Sum).Sum
}

function Get-CacheMB {

    [math]::Round(
        (Get-CacheSize) / 1MB,
        2
    )
}

function Clean-Cache {

    $limit =
        $Config.CacheLimitMB * 1MB

    while ((Get-CacheSize) -gt $limit) {

        $oldest =
            Get-ChildItem $Config.CacheDir -File |
            Sort-Object LastAccessTime |
            Select-Object -First 1

        if (!$oldest) {
            break
        }

        Write-Host "Removing cache $($oldest.Name)"

        Remove-Item $oldest.FullName -Force
    }
}

function Get-CacheFile {

    param(
        [string]$VideoId
    )

    Get-ChildItem `
        $Config.CacheDir `
        -File `
        -ErrorAction SilentlyContinue |
    Where-Object {
        $_.BaseName -eq $VideoId
    } |
    Select-Object -First 1
}

# ============================================================
# YT-DLP
# ============================================================

function Search-Youtube {

    param(
        [string]$Keyword
    )

    Write-Host ""
    Write-Host "Searching: $Keyword"

    $result =
        & $Config.YtDlp `
            --js-runtimes $Config.JsRuntime `
            --flat-playlist `
            --dump-single-json `
            "ytsearch20:$Keyword"

    if ($LASTEXITCODE -ne 0 -or !$result) {
        Write-Host "Search failed. yt-dlp exit code: $LASTEXITCODE"
        return @()
    }

    try {
        $json = $result | ConvertFrom-Json
    }
    catch {
        Write-Host "YouTube returned invalid search data"
        return @()
    }

    if (!$json.entries) {
        Write-Host "No search results found"
        return @()
    }

    $list = @()

    foreach ($entry in $json.entries) {

        $list += [PSCustomObject]@{
            Id    = $entry.id
            Title = $entry.title
        }
    }

    return $list
}

function Get-YoutubePlaylistSongs {

    param(
        [string]$Url
    )

    $result =
        & $Config.YtDlp `
            --js-runtimes $Config.JsRuntime `
            --flat-playlist `
            --dump-single-json `
            $Url

    if ($LASTEXITCODE -ne 0 -or !$result) {
        Write-Host "Failed to load playlist. yt-dlp exit code: $LASTEXITCODE"
        return @()
    }

    try {
        $json = $result | ConvertFrom-Json
    }
    catch {
        Write-Host "yt-dlp returned invalid playlist data"
        return @()
    }

    if (!$json.entries) {
        Write-Host "No playlist entries found"
        return @()
    }

    $songs = @()

    foreach ($entry in $json.entries) {
        if (!$entry.id -or !$entry.title) {
            continue
        }

        $songs += [PSCustomObject]@{
            Id = [string]$entry.id
            Title = [string]$entry.title
        }
    }

    return $songs
}

function Save-CurrentPlaylist {

    $playlistFile =
        Join-Path `
            $Config.DataDir `
            "playlist.json"

    $script:CurrentPlaylist |
        ConvertTo-Json -Depth 5 |
        Set-Content $playlistFile
}

function Load-YoutubePlaylist {

    param(
        [string]$Url
    )

    $Url = ($Url -replace '[\x00-\x1F\x7F]', '').Trim()

    $parsedUrl = $null

    if (
        ![uri]::TryCreate($Url, [System.UriKind]::Absolute, [ref]$parsedUrl) -or
        $parsedUrl.Scheme -notin @("http", "https")
    ) {
        Write-Host "Invalid playlist URL"
        return
    }

    Write-Host ""
    Write-Host "Loading playlist..."

    $songs = @(Get-YoutubePlaylistSongs $Url)

    if ($songs.Count -eq 0) {
        return
    }

    $script:CurrentPlaylist = $songs

    $script:CurrentIndex = -1
    $script:CurrentSong = $null
    Save-CurrentPlaylist

    Write-Host ""
    Write-Host "Playlist loaded:"
    Write-Host $script:CurrentPlaylist.Count "songs"
}

function Resolve-YoutubeProfilePlaylistsUrl {

    param(
        [string]$Profile
    )

    $Profile = ($Profile -replace '[\x00-\x1F\x7F]', '').Trim()

    if ($Profile -match '^@[\w.-]+$') {
        return "https://www.youtube.com/$Profile/playlists"
    }

    $parsedUrl = $null

    if (
        ![uri]::TryCreate($Profile, [System.UriKind]::Absolute, [ref]$parsedUrl) -or
        $parsedUrl.Scheme -notin @("http", "https") -or
        $parsedUrl.Host -notmatch '(^|\.)youtube\.com$'
    ) {
        return $null
    }

    $builder = [System.UriBuilder]$parsedUrl
    $builder.Query = ""
    $builder.Fragment = ""
    $builder.Path = $builder.Path.TrimEnd("/")

    if ($builder.Path -notmatch '/playlists$') {
        $builder.Path += "/playlists"
    }

    return $builder.Uri.AbsoluteUri
}

function Find-ProfilePlaylists {

    param(
        [string]$Profile
    )

    $profileUrl = Resolve-YoutubeProfilePlaylistsUrl $Profile

    if (!$profileUrl) {
        Write-Host "Invalid YouTube profile. Use @handle or a YouTube channel URL."
        return @()
    }

    Write-Host ""
    Write-Host "Loading public playlists from:"
    Write-Host $profileUrl

    $result =
        & $Config.YtDlp `
            --js-runtimes $Config.JsRuntime `
            --flat-playlist `
            --dump-single-json `
            $profileUrl

    if ($LASTEXITCODE -ne 0 -or !$result) {
        Write-Host "Failed to load profile playlists. yt-dlp exit code: $LASTEXITCODE"
        return @()
    }

    try {
        $json = $result | ConvertFrom-Json
    }
    catch {
        Write-Host "yt-dlp returned invalid profile data"
        return @()
    }

    $playlists = @()

    foreach ($entry in @($json.entries)) {
        if (!$entry.id -or !$entry.title) {
            continue
        }

        $url = $null

        foreach ($propertyName in @("webpage_url", "original_url", "url")) {
            if (
                $entry.PSObject.Properties.Name -contains $propertyName -and
                $entry.$propertyName
            ) {
                $url = [string]$entry.$propertyName
                break
            }
        }

        if (!$url -or $url -notmatch '^https?://') {
            $url = "https://www.youtube.com/playlist?list=$($entry.id)"
        }

        $playlists += [PSCustomObject]@{
            Id = [string]$entry.id
            Title = [string]$entry.title
            Url = $url
        }
    }

    return $playlists
}

function Get-YoutubeRecommendation {

    param(
        [string]$VideoId
    )

    if (!$VideoId) {
        return $null
    }

    Write-Host ""
    Write-Host "Loading YouTube recommendations..."

    $mixUrl =
        "https://www.youtube.com/watch?v=$VideoId&list=RD$VideoId"

    $result =
        & $Config.YtDlp `
            --js-runtimes $Config.JsRuntime `
            --flat-playlist `
            --playlist-end 25 `
            --dump-single-json `
            $mixUrl

    if ($LASTEXITCODE -ne 0 -or !$result) {
        Write-Host "Could not load YouTube recommendations"
        return $null
    }

    try {
        $json = $result | ConvertFrom-Json
    }
    catch {
        Write-Host "YouTube returned invalid recommendation data"
        return $null
    }

    $knownIds = @($script:CurrentPlaylist | ForEach-Object { $_.Id })

    $recommendation =
        $json.entries |
        Where-Object {
            $_.id -and
            $_.title -and
            $_.id -ne $VideoId -and
            $_.id -notin $knownIds
        } |
        Select-Object -First 1

    if (!$recommendation) {
        Write-Host "No new YouTube recommendation found"
        return $null
    }

    return [PSCustomObject]@{
        Id = $recommendation.id
        Title = $recommendation.title
    }
}

function Get-StreamUrl {

    param(
        [string]$VideoId
    )

    $url =
        "https://www.youtube.com/watch?v=$VideoId"

    & $Config.YtDlp `
        --js-runtimes $Config.JsRuntime `
        -f ba `
        -g `
        $url |
    Select-Object -First 1
}

function Start-BackgroundCache {

    param(
        [string]$VideoId
    )

    $cached =
        Get-CacheFile $VideoId

    if ($cached) {
        return
    }

    $url =
        "https://www.youtube.com/watch?v=$VideoId"

    Start-Process `
        -FilePath $Config.YtDlp `
        -ArgumentList @(
            "--js-runtimes",$Config.JsRuntime,
            "-f","ba",
            "-o",
            "$($Config.CacheDir)\$VideoId.%(ext)s",
            $url
        ) `
        -WindowStyle Hidden
}

# ============================================================
# HELPERS
# ============================================================

function Show-Songs {

    param(
        [array]$Songs
    )

    $i = 1

    foreach ($song in $Songs) {

        "{0,3}. {1}" -f $i,$song.Title

        $i++
    }
}

function Get-CurrentSongOrWarn {

    if (!$script:CurrentSong) {
        Write-Host "Nothing is playing"
        return $null
    }

    return $script:CurrentSong
}

function Get-ThumbnailFile {

    param(
        [string]$VideoId
    )

    $thumbnailFile =
        Join-Path $Config.CacheDir "$VideoId.thumbnail.jpg"

    if (Test-Path -LiteralPath $thumbnailFile) {
        return $thumbnailFile
    }

    $thumbnailUrl =
        "https://i.ytimg.com/vi/$VideoId/hqdefault.jpg"

    try {
        Invoke-WebRequest `
            -Uri $thumbnailUrl `
            -OutFile $thumbnailFile `
            -TimeoutSec 10

        return $thumbnailFile
    }
    catch {
        Remove-Item `
            -LiteralPath $thumbnailFile `
            -Force `
            -ErrorAction SilentlyContinue

        return $null
    }
}

function Show-Thumbnail {

    $song = Get-CurrentSongOrWarn

    if (!$song) {
        return
    }

    $thumbnailFile = Get-ThumbnailFile $song.Id

    if (!$thumbnailFile) {
        Write-Host "Could not load thumbnail"
        return
    }

    try {
        Add-Type -AssemblyName System.Drawing

        $source =
            [System.Drawing.Image]::FromFile($thumbnailFile)

        try {
            $width = [math]::Max(
                8,
                [math]::Min(
                    [int]$Config.ThumbnailWidth,
                    [math]::Max(8, [Console]::WindowWidth - 2)
                )
            )

            # A terminal character is roughly twice as tall as it is wide.
            $pixelHeight = [math]::Max(
                2,
                [int][math]::Round(
                    ($source.Height / $source.Width) * $width
                )
            )

            if (($pixelHeight % 2) -ne 0) {
                $pixelHeight++
            }

            $bitmap = $null
            $bitmap =
                [System.Drawing.Bitmap]::new(
                    $width,
                    $pixelHeight
                )

            try {
                $graphics =
                    [System.Drawing.Graphics]::FromImage($bitmap)

                try {
                    $graphics.InterpolationMode =
                        [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBilinear

                    $graphics.DrawImage(
                        $source,
                        0,
                        0,
                        $width,
                        $pixelHeight
                    )
                }
                finally {
                    $graphics.Dispose()
                }

                $escape = [char]27

                Write-Host ""
                Write-Host "Thumbnail:" -ForegroundColor Yellow

                for ($y = 0; $y -lt $pixelHeight; $y += 2) {
                    $line =
                        [System.Text.StringBuilder]::new()

                    for ($x = 0; $x -lt $width; $x++) {
                        $top = $bitmap.GetPixel($x, $y)
                        $bottom = $bitmap.GetPixel($x, $y + 1)

                        [void]$line.Append(
                            "$escape[38;2;$($top.R);$($top.G);$($top.B)m" +
                            "$escape[48;2;$($bottom.R);$($bottom.G);$($bottom.B)m" +
                            [char]0x2580
                        )
                    }

                    [void]$line.Append("$escape[0m")
                    [Console]::WriteLine($line.ToString())
                }
            }
            finally {
                if ($bitmap) {
                    $bitmap.Dispose()
                }
            }
        }
        finally {
            $source.Dispose()
        }
    }
    catch {
        try {
            [Console]::Write("$([char]27)[0m")
        }
        catch {
        }

        Write-Host "Could not render color thumbnail: $($_.Exception.Message)"
    }
}

function Get-LyricsSearchTitle {

    param(
        [string]$Title
    )

    $cleanTitle =
        $Title -replace
            '(?i)\s*[\(\[][^(\[]*(official|audio|video|lyrics?|visuali[sz]er|live)[^)\]]*[\)\]]',
            ''

    return (
        ($cleanTitle -replace '\s+', ' ').Trim()
    )
}

function Show-Lyrics {

    $song = Get-CurrentSongOrWarn

    if (!$song) {
        return
    }

    $query = Get-LyricsSearchTitle $song.Title

    Write-Host ""
    Write-Host "Lyrics: $query" -ForegroundColor Yellow
    Write-Host "Searching LRCLIB..." -ForegroundColor DarkGray

    try {
        $uri =
            "$($Config.LyricsApi)?q=$([uri]::EscapeDataString($query))"

        $results =
            Invoke-RestMethod `
                -Uri $uri `
                -Headers @{
                    "User-Agent" = "ytmusic-cli/1.0"
                } `
                -TimeoutSec 10

        $match = $null

        foreach ($result in $results) {
            if ($result.plainLyrics -or $result.syncedLyrics) {
                $match = $result
                break
            }
        }

        if (!$match) {
            Write-Host "Lyrics not found"
            return
        }

        $lyrics = [string]$match.plainLyrics

        if (!$lyrics) {
            $lyrics =
                ([string]$match.syncedLyrics) -replace
                    '(?m)^\[(?:\d{2}:)?\d{2}:\d{2}(?:\.\d+)?\]\s*',
                    ''
        }

        Write-Host ""
        Write-Host (
            "$($match.trackName) - $($match.artistName)"
        ) -ForegroundColor Cyan
        Write-Host ""
        Write-Host $lyrics.Trim()
        Write-Host ""
        Write-Host "Lyrics provided by LRCLIB" -ForegroundColor DarkGray
    }
    catch {
        Write-Host "Could not load lyrics: $($_.Exception.Message)"
    }
}

function Show-NowPlayingArt {

    Show-Thumbnail
    Show-Lyrics
}

function Show-PlaylistWindow {

    param(
        [ValidateSet("Up", "Down")]
        [string]$Direction
    )

    if ($script:CurrentPlaylist.Count -eq 0) {
        Write-Host "Playlist is empty"
        return
    }

    $windowSize = 15
    $anchor =
        if ($script:CurrentIndex -ge 0) {
            $script:CurrentIndex
        }
        else {
            0
        }

    if ($Direction -eq "Up") {
        $start = [math]::Max(0, $anchor - ($windowSize - 1))
    }
    else {
        $start = $anchor
    }

    $end = [math]::Min(
        $script:CurrentPlaylist.Count - 1,
        $start + ($windowSize - 1)
    )

    Write-Host ""
    Write-Host "Playlist ($($start + 1)-$($end + 1) of $($script:CurrentPlaylist.Count)):" -ForegroundColor Yellow

    for ($index = $start; $index -le $end; $index++) {
        $marker =
            if ($index -eq $script:CurrentIndex) { ">" } else { " " }

        "{0} {1,3}. {2}" -f
            $marker,
            ($index + 1),
            $script:CurrentPlaylist[$index].Title
    }
}

function Get-PlaylistLibraryFile {

    Join-Path $Config.PlaylistLibraryDir "library.json"
}

function Save-PlaylistLibrary {

    $script:PlaylistLibrary |
        ConvertTo-Json -Depth 5 |
        Set-Content (Get-PlaylistLibraryFile)
}

function Restore-PlaylistLibrary {

    $libraryFile = Get-PlaylistLibraryFile

    if (!(Test-Path $libraryFile)) {
        $script:PlaylistLibrary = @()
        return
    }

    try {
        $script:PlaylistLibrary =
            @(Get-Content $libraryFile -Raw | ConvertFrom-Json)
    }
    catch {
        $script:PlaylistLibrary = @()
        Write-Host "Could not restore local playlist library"
    }
}

function Show-PlaylistManager {

    Restore-PlaylistLibrary

    Write-Host ""
    Write-Host "Local Playlist Manager:" -ForegroundColor Yellow

    if ($script:PlaylistLibrary.Count -eq 0) {
        Write-Host "No local playlists. Use profile add <number|all> first."
        return
    }

    for ($index = 0; $index -lt $script:PlaylistLibrary.Count; $index++) {
        $playlist = $script:PlaylistLibrary[$index]

        "{0,3}. {1} ({2} songs)" -f
            ($index + 1),
            $playlist.Title,
            $playlist.SongCount
    }

    Write-Host ""
    Write-Host "Use: playlist play <number>" -ForegroundColor Yellow
    Write-Host "Or : playlist use <number> (load without playing)" -ForegroundColor Yellow
}

function Import-ProfilePlaylist {

    param(
        [int]$Index
    )

    if (
        $Index -lt 0 -or
        $Index -ge $script:ProfilePlaylistResults.Count
    ) {
        Write-Host "Invalid profile playlist number"
        return $false
    }

    $playlist = $script:ProfilePlaylistResults[$Index]

    Write-Host ""
    Write-Host "Importing playlist:"
    Write-Host $playlist.Title

    $songs = @(Get-YoutubePlaylistSongs $playlist.Url)

    if ($songs.Count -eq 0) {
        Write-Host "Playlist was not added"
        return $false
    }

    $safeId = $playlist.Id -replace '[^A-Za-z0-9_-]', '_'

    if (!$safeId) {
        $safeId = [guid]::NewGuid().ToString("N")
    }

    $fileName = "$safeId.json"
    $playlistFile = Join-Path $Config.PlaylistLibraryDir $fileName

    $songs |
        ConvertTo-Json -Depth 5 |
        Set-Content $playlistFile

    Restore-PlaylistLibrary

    $existingIndex = -1

    for ($i = 0; $i -lt $script:PlaylistLibrary.Count; $i++) {
        if ($script:PlaylistLibrary[$i].Id -eq $playlist.Id) {
            $existingIndex = $i
            break
        }
    }

    $libraryEntry = [PSCustomObject]@{
        Id = $playlist.Id
        Title = $playlist.Title
        Url = $playlist.Url
        File = $fileName
        SongCount = $songs.Count
        ImportedAt = (Get-Date).ToString("o")
    }

    if ($existingIndex -ge 0) {
        $updatedLibrary = @()

        for ($i = 0; $i -lt $script:PlaylistLibrary.Count; $i++) {
            if ($i -eq $existingIndex) {
                $updatedLibrary += $libraryEntry
            }
            else {
                $updatedLibrary += $script:PlaylistLibrary[$i]
            }
        }

        $script:PlaylistLibrary = $updatedLibrary
        Write-Host "Updated local playlist: $($songs.Count) songs"
    }
    else {
        $script:PlaylistLibrary += $libraryEntry
        Write-Host "Added local playlist: $($songs.Count) songs"
    }

    Save-PlaylistLibrary
    return $true
}

function Import-AllProfilePlaylists {

    if ($script:ProfilePlaylistResults.Count -eq 0) {
        Write-Host "No profile playlist results. Use profile load <profile> first."
        return
    }

    $added = 0

    for ($index = 0; $index -lt $script:ProfilePlaylistResults.Count; $index++) {
        Write-Host ""
        Write-Host (
            "[{0}/{1}]" -f
                ($index + 1),
                $script:ProfilePlaylistResults.Count
        )

        if (Import-ProfilePlaylist $index) {
            $added++
        }
    }

    Write-Host ""
    Write-Host "Import complete: $added of $($script:ProfilePlaylistResults.Count) playlists added"
}

function Use-LocalPlaylist {

    param(
        [int]$Index,
        [switch]$Play
    )

    Restore-PlaylistLibrary

    if (
        $Index -lt 0 -or
        $Index -ge $script:PlaylistLibrary.Count
    ) {
        Write-Host "Invalid local playlist number"
        return
    }

    $playlist = $script:PlaylistLibrary[$Index]
    $playlistFile = Join-Path $Config.PlaylistLibraryDir $playlist.File

    if (!(Test-Path $playlistFile)) {
        Write-Host "Local playlist file is missing: $($playlist.File)"
        return
    }

    try {
        $songs = @(Get-Content $playlistFile -Raw | ConvertFrom-Json)
    }
    catch {
        Write-Host "Could not read local playlist"
        return
    }

    if ($songs.Count -eq 0) {
        Write-Host "Local playlist is empty"
        return
    }

    $script:CurrentPlaylist = $songs
    $script:CurrentIndex = -1
    $script:CurrentSong = $null
    $script:Queue.Clear()
    Save-CurrentPlaylist
    Save-State

    Write-Host ""
    Write-Host "Selected playlist:"
    Write-Host "$($playlist.Title) ($($songs.Count) songs)"

    if ($Play) {
        Play-Song 0
    }
}

function Remove-LocalPlaylist {

    param(
        [int]$Index
    )

    Restore-PlaylistLibrary

    if (
        $Index -lt 0 -or
        $Index -ge $script:PlaylistLibrary.Count
    ) {
        Write-Host "Invalid local playlist number"
        return
    }

    $playlist = $script:PlaylistLibrary[$Index]

    Write-Host ""
    $confirmation =
        Read-Host "Delete '$($playlist.Title)' from local library? (y/N)"

    if ($confirmation -notmatch '^(?i:y|yes)$') {
        Write-Host "Delete cancelled"
        return
    }

    $playlistFile = Join-Path $Config.PlaylistLibraryDir $playlist.File

    if (Test-Path $playlistFile) {
        Remove-Item -LiteralPath $playlistFile -Force
    }

    $updatedLibrary = @()

    for ($i = 0; $i -lt $script:PlaylistLibrary.Count; $i++) {
        if ($i -ne $Index) {
            $updatedLibrary += $script:PlaylistLibrary[$i]
        }
    }

    $script:PlaylistLibrary = $updatedLibrary
    Save-PlaylistLibrary

    Write-Host "Deleted local playlist: $($playlist.Title)"
}

function Save-State {

    @{
        Shuffle = $script:Shuffle
        AutoRecommend = $script:AutoRecommend
        CurrentIndex = $script:CurrentIndex
    } |
    ConvertTo-Json |
    Set-Content (
        Join-Path $Config.DataDir "state.json"
    )
}

function Restore-State {

    $stateFile =
        Join-Path $Config.DataDir "state.json"

    if (!(Test-Path $stateFile)) {
        return
    }

    try {
        $state = Get-Content $stateFile -Raw | ConvertFrom-Json

        if ($state.PSObject.Properties.Name -contains "Shuffle") {
            $script:Shuffle = [bool]$state.Shuffle
        }

        if ($state.PSObject.Properties.Name -contains "AutoRecommend") {
            $script:AutoRecommend = [bool]$state.AutoRecommend
        }
    }
    catch {
        Write-Host "Could not restore saved player state"
    }
}

function Restore-Playlist {

    $playlistFile =
        Join-Path $Config.DataDir "playlist.json"

    if (!(Test-Path $playlistFile)) {
        return
    }

    try {
        $script:CurrentPlaylist =
            @(Get-Content $playlistFile -Raw | ConvertFrom-Json)

        if ($script:CurrentPlaylist.Count -gt 0) {
            Write-Host "Playlist restored: $($script:CurrentPlaylist.Count) songs"
        }
    }
    catch {
        Write-Host "Could not restore saved playlist"
    }
}

function Show-Help {

    Write-Host ""
    Write-Host "Commands:" -ForegroundColor Yellow
    Write-Host "  playlist load <url>  Load YouTube playlist"
    Write-Host "  playlist show        Show loaded playlist"
    Write-Host "  profile load <url>   Find all public playlists from a profile"
    Write-Host "  profile show         Show the last profile playlist results"
    Write-Host "  profile add <n>      Add one profile playlist to local library"
    Write-Host "  profile add all      Add all profile playlists to local library"
    Write-Host "  playlist manager     Show local playlist library"
    Write-Host "  playlist use <n>     Select a local playlist"
    Write-Host "  playlist play <n>    Select and play a local playlist"
    Write-Host "  playlist delete <n>  Delete a local playlist"
    Write-Host "  search <keyword>     Search YouTube"
    Write-Host "  plays <n>            Play a search result"
    Write-Host "  queues <n>           Add a search result to queue"
    Write-Host "  play <n>             Play a playlist song"
    Write-Host "  queue <n>            Add song to queue"
    Write-Host "  queue                Show queue"
    Write-Host "  pause / resume       Pause or resume playback"
    Write-Host "  next / prev          Change song"
    Write-Host "  stop                 Stop playback"
    Write-Host "  status               Show playback details"
    Write-Host "  lyrics               Show lyrics for current song"
    Write-Host "  thumbnail            Show color ASCII thumbnail"
    Write-Host "  now                  Show thumbnail and lyrics"
    Write-Host "  shuffle on/off       Toggle shuffle"
    Write-Host "  autorec on/off       Toggle YouTube recommendations"
    Write-Host "  cache                Show cache usage"
    Write-Host "  help                 Show this help"
    Write-Host "  quit                 Exit player"
    Write-Host ""
    Write-Host "Keys:" -ForegroundColor Yellow
    Write-Host "  Space  Play/Pause"
    Write-Host "  Left   Previous song"
    Write-Host "  Right  Next song"
    Write-Host "  Up     Show previous playlist items"
    Write-Host "  Down   Show next playlist items"
    Write-Host "  F1     Show help"
    Write-Host "  F8     Toggle Auto Recommendation"
    Write-Host ""
}

# ============================================================
# BOOT
# ============================================================

Start-VLC

Clean-Cache

Restore-Playlist

Restore-State

Restore-PlaylistLibrary

Write-Host ""
Write-Host "========================================"
Write-Host " YouTube Music CLI"
Write-Host "========================================"
Write-Host "VLC HTTP port: $($Config.HttpPort)"

Show-Help

# ============================================================
# PLAYBACK
# ============================================================

function Play-Stream {

    param(
        [string]$Url
    )

    $encoded =
        [uri]::EscapeDataString($Url)

    Invoke-VLCCommand "pl_empty" | Out-Null

    Invoke-VLCCommand "in_play&input=$encoded" | Out-Null
}

function Play-Song {

    param(
        [int]$Index
    )

    if (
        $Index -lt 0 -or
        $Index -ge $script:CurrentPlaylist.Count
    ) {
        Write-Host "Invalid song number"
        return
    }

    $song =
        $script:CurrentPlaylist[$Index]

    $script:CurrentSong = $song
    $script:CurrentIndex = $Index
    $script:AutoAdvanceArmed = $false

    Write-Host ""
    Write-Host "Now Playing:"
    Write-Host $song.Title

    $cached =
        Get-CacheFile $song.Id

    if ($cached) {

        $cached.LastAccessTime = Get-Date

        Write-Host "[CACHE]"

        Play-Stream $cached.FullName
    }
    else {

        Write-Host "[STREAM]"

        $stream =
            Get-StreamUrl $song.Id

        if (!$stream) {

            Write-Host "Failed to get stream"
            return
        }

        Play-Stream $stream

        Start-BackgroundCache $song.Id
    }

    Start-PreloadNext

    Save-State
}

function Pause-Song {

    $status = Get-VLCStatus

    if ($status.state -eq "playing") {
        Invoke-VLCCommand "pl_pause" | Out-Null
        Write-Host "Paused: $($script:CurrentSong.Title)"
    }
    elseif ($status.state -eq "paused") {
        Write-Host "Already paused"
    }
    else {
        Write-Host "Nothing is playing"
    }
}

function Resume-Song {

    $status = Get-VLCStatus

    if ($status.state -eq "paused") {
        Invoke-VLCCommand "pl_pause" | Out-Null
        Write-Host "Playing: $($script:CurrentSong.Title)"
    }
    elseif ($status.state -eq "playing") {
        Write-Host "Already playing"
    }
    else {
        Write-Host "Nothing to resume"
    }
}

function Stop-Song {

    $script:AutoAdvanceArmed = $false

    Invoke-VLCCommand "pl_stop" | Out-Null

    Write-Host "Stopped"
}

# ============================================================
# QUEUE
# ============================================================

function Add-ToQueue {

    param(
        [int]$Index
    )

    if (
        $Index -lt 0 -or
        $Index -ge $script:CurrentPlaylist.Count
    ) {
        return
    }

    $song =
        $script:CurrentPlaylist[$Index]

    [void]$script:Queue.Add($song)

    Write-Host ""
    Write-Host "Queued:"
    Write-Host $song.Title
}

function Show-Queue {

    if ($script:Queue.Count -eq 0) {

        Write-Host "Queue empty"
        return
    }

    Write-Host ""

    $n = 1

    foreach ($song in $script:Queue) {

        "{0,3}. {1}" -f $n,$song.Title

        $n++
    }
}

# ============================================================
# NEXT / PREV
# ============================================================

function Get-NextIndex {

    if ($script:Queue.Count -gt 0) {

        $song =
            $script:Queue[0]

        $script:Queue.RemoveAt(0)

        return (
            $script:CurrentPlaylist.IndexOf($song)
        )
    }

    if ($script:Shuffle) {

        return (
            Get-Random `
                -Minimum 0 `
                -Maximum $script:CurrentPlaylist.Count
        )
    }

    if (
        $script:AutoRecommend -and
        $script:CurrentIndex -ge ($script:CurrentPlaylist.Count - 1)
    ) {
        $recommendation =
            Get-YoutubeRecommendation $script:CurrentSong.Id

        if ($recommendation) {
            $script:CurrentPlaylist += $recommendation

            Write-Host "Recommended:"
            Write-Host $recommendation.Title

            return ($script:CurrentPlaylist.Count - 1)
        }
    }

    return (
        ($script:CurrentIndex + 1) %
        $script:CurrentPlaylist.Count
    )
}

function Next-Song {

    if ($script:CurrentPlaylist.Count -eq 0) {
        return
    }

    $next =
        Get-NextIndex

    Play-Song $next
}

function Prev-Song {

    if ($script:CurrentPlaylist.Count -eq 0) {
        return
    }

    $prev =
        $script:CurrentIndex - 1

    if ($prev -lt 0) {

        $prev =
            $script:CurrentPlaylist.Count - 1
    }

    Play-Song $prev
}

# ============================================================
# PRELOAD
# ============================================================

function Start-PreloadNext {

    if (
        $script:CurrentPlaylist.Count -eq 0
    ) {
        return
    }

    $next =
        ($script:CurrentIndex + 1)

    if (
        $next -ge
        $script:CurrentPlaylist.Count
    ) {
        return
    }

    $song =
        $script:CurrentPlaylist[$next]

    $cached =
        Get-CacheFile $song.Id

    if ($cached) {
        return
    }

    Write-Host ""
    Write-Host "Preloading:"
    Write-Host $song.Title

    Start-BackgroundCache $song.Id
}

# ============================================================
# STATUS
# ============================================================

function Show-Status {

    try {

        $status =
            Get-VLCStatus

        Write-Host ""

        if ($script:CurrentSong) {

            Write-Host "Song :"
            Write-Host $script:CurrentSong.Title
        }

        Write-Host ""

        Write-Host "State :"
        Write-Host $status.state

        Write-Host ""

        Write-Host "Volume :"
        Write-Host $status.volume

        Write-Host ""

        Write-Host "Position :"
        Write-Host (
            [math]::Round(
                $status.position * 100,
                1
            )
        ) "%"
    }
    catch {

        Write-Host "No active playback"
    }
}

function Test-AutoNext {

    if (!$script:CurrentSong -or $script:CurrentPlaylist.Count -eq 0) {
        return $false
    }

    try {
        $status = Get-VLCStatus

        if ($status.state -eq "playing") {
            $script:AutoAdvanceArmed = $true
            return $false
        }

        if ($status.state -eq "stopped" -and $script:AutoAdvanceArmed) {
            $script:AutoAdvanceArmed = $false
            Write-Host ""
            Write-Host "Song finished. Playing next..."
            Next-Song
            return $true
        }
    }
    catch {
    }

    return $false
}

function Read-PlayerCommand {

    $statusRow = [Console]::CursorTop
    Write-Host ""
    Write-Host -NoNewline "ytmusic: "
    $inputBuffer = ""
    $lastStatusCheck = [datetime]::MinValue

    while ($true) {
        if ([Console]::KeyAvailable) {
            $key = [Console]::ReadKey($true)

            if ($key.Key -eq [ConsoleKey]::Enter) {
                Write-Host ""
                return (($inputBuffer -replace '[\x00-\x1F\x7F]', '').Trim())
            }

            if ($inputBuffer.Length -eq 0) {
                if ($key.Key -eq [ConsoleKey]::F1) {
                    Write-Host ""
                    return "help"
                }

                if ($key.Key -eq [ConsoleKey]::F8) {
                    Write-Host ""
                    return "__toggle_autorecommend"
                }

                if ($key.Key -eq [ConsoleKey]::Spacebar) {
                    Write-Host ""
                    return "__toggle_playback"
                }

                if ($key.Key -eq [ConsoleKey]::LeftArrow) {
                    Write-Host ""
                    return "prev"
                }

                if ($key.Key -eq [ConsoleKey]::RightArrow) {
                    Write-Host ""
                    return "next"
                }

                if ($key.Key -eq [ConsoleKey]::UpArrow) {
                    Write-Host ""
                    return "__playlist_up"
                }

                if ($key.Key -eq [ConsoleKey]::DownArrow) {
                    Write-Host ""
                    return "__playlist_down"
                }
            }

            if ($key.Key -eq [ConsoleKey]::Backspace) {
                if ($inputBuffer.Length -gt 0) {
                    $inputBuffer = $inputBuffer.Substring(0, $inputBuffer.Length - 1)
                    Write-Host -NoNewline "`b `b"
                }
                continue
            }

            if (
                $key.KeyChar -ne [char]0 -and
                ![char]::IsControl($key.KeyChar)
            ) {
                $inputBuffer += $key.KeyChar
                Write-Host -NoNewline $key.KeyChar
            }
        }
        else {
            Start-Sleep -Milliseconds 100
        }

        if (((Get-Date) - $lastStatusCheck).TotalMilliseconds -ge 750) {
            $lastStatusCheck = Get-Date

            if ($inputBuffer.Length -eq 0) {
                Write-TuiStatus $statusRow
            }

            if (Test-AutoNext) {
                Write-Host -NoNewline "ytmusic: $inputBuffer"
            }
        }
    }
}

# ============================================================
# CACHE COMMAND
# ============================================================

function Show-Cache {

    Write-Host ""

    Write-Host "Cache Usage:"
    Write-Host (
        "$(Get-CacheMB) MB / $($Config.CacheLimitMB) MB"
    )

    Write-Host ""

    Get-ChildItem `
        $Config.CacheDir `
        -File |
    Sort-Object Length -Descending |
    Select-Object `
        Name,
        @{
            N='MB'
            E={
                [math]::Round(
                    $_.Length/1MB,
                    2
                )
            }
        } |
    Format-Table
}

# ============================================================
# PLAY SEARCH RESULT
# ============================================================

$script:SearchResult = @()

function Search-Command {

    param(
        [string]$Keyword
    )

    $script:SearchResult =
        @(Search-Youtube $Keyword)

    Show-Songs $script:SearchResult

    if ($script:SearchResult.Count -gt 0) {
        Write-Host ""
        Write-Host "Use: plays <number>" -ForegroundColor Yellow
        Write-Host "Or : queues <number>" -ForegroundColor Yellow
    }
}

function Show-ProfilePlaylistResults {

    Write-Host ""
    Write-Host "Profile Public Playlists:" -ForegroundColor Yellow

    if ($script:ProfilePlaylistResults.Count -eq 0) {
        Write-Host "No profile playlist results"
        return
    }

    for ($index = 0; $index -lt $script:ProfilePlaylistResults.Count; $index++) {
        "{0,3}. {1}" -f
            ($index + 1),
            $script:ProfilePlaylistResults[$index].Title
    }

    Write-Host ""
    Write-Host "Use: profile add <number>" -ForegroundColor Yellow
    Write-Host "Or : profile add all" -ForegroundColor Yellow
}

function Load-ProfilePlaylistResults {

    param(
        [string]$Profile
    )

    $script:ProfilePlaylistResults =
        @(Find-ProfilePlaylists $Profile)

    Show-ProfilePlaylistResults
}

function Play-SearchResult {

    param(
        [int]$Index
    )

    if (
        $Index -lt 0 -or
        $Index -ge $script:SearchResult.Count
    ) {
        Write-Host "Invalid search result number"
        return
    }

    $song =
        $script:SearchResult[$Index]

    $existingIndex = -1

    for ($i = 0; $i -lt $script:CurrentPlaylist.Count; $i++) {
        if ($script:CurrentPlaylist[$i].Id -eq $song.Id) {
            $existingIndex = $i
            break
        }
    }

    if ($existingIndex -ge 0) {
        Play-Song $existingIndex
    }
    else {
        $script:CurrentPlaylist += $song
        Play-Song ($script:CurrentPlaylist.Count - 1)
    }
}

function Add-SearchResultToQueue {

    param(
        [int]$Index
    )

    if (
        $Index -lt 0 -or
        $Index -ge $script:SearchResult.Count
    ) {
        Write-Host "Invalid search result number"
        return
    }

    $song = $script:SearchResult[$Index]
    $playlistIndex = -1

    for ($i = 0; $i -lt $script:CurrentPlaylist.Count; $i++) {
        if ($script:CurrentPlaylist[$i].Id -eq $song.Id) {
            $playlistIndex = $i
            break
        }
    }

    if ($playlistIndex -lt 0) {
        $script:CurrentPlaylist += $song
        $playlistIndex = $script:CurrentPlaylist.Count - 1
        Save-CurrentPlaylist
    }

    Add-ToQueue $playlistIndex
}

# ============================================================
# COMMAND LOOP
# ============================================================

try {
    :PlayerLoop while ($true) {

        Write-Host ""

        $cmd =
            Read-PlayerCommand

        if (!$cmd) {
            continue
        }

        switch -Regex ($cmd) {

            '^quit$' {

                $script:AutoAdvanceArmed = $false
                break PlayerLoop
            }

        '^help$' {

            Show-Help
        }

        '^__playlist_up$' {

            Show-PlaylistWindow "Up"
        }

        '^__playlist_down$' {

            Show-PlaylistWindow "Down"
        }

        '^__toggle_autorecommend$' {

            $script:AutoRecommend = !$script:AutoRecommend
            Save-State

            $state =
                if ($script:AutoRecommend) { "ON" } else { "OFF" }

            Write-Host "YouTube Auto Recommendation $state"
        }

        '^__toggle_playback$' {

            try {
                $status = Get-VLCStatus

                if ($status.state -eq "playing") {
                    Pause-Song
                }
                elseif ($status.state -eq "paused") {
                    Resume-Song
                }
                elseif ($script:CurrentIndex -ge 0) {
                    Play-Song $script:CurrentIndex
                }
                elseif ($script:CurrentPlaylist.Count -gt 0) {
                    Play-Song 0
                }
                else {
                    Write-Host "Playlist is empty"
                }
            }
            catch {
                Write-Host "VLC is not responding"
            }
        }

        '^playlist load (.+)$' {

            Load-YoutubePlaylist $Matches[1]
        }

        '^profile load (.+)$' {

            Load-ProfilePlaylistResults $Matches[1]
        }

        '^profile show$' {

            Show-ProfilePlaylistResults
        }

        '^profile add all$' {

            Import-AllProfilePlaylists
        }

        '^profile add (\d+)$' {

            [void](Import-ProfilePlaylist (
                [int]$Matches[1] - 1
            ))
        }

        '^playlist manager$' {

            Show-PlaylistManager
        }

        '^playlist use (\d+)$' {

            Use-LocalPlaylist (
                [int]$Matches[1] - 1
            )
        }

        '^playlist play (\d+)$' {

            Use-LocalPlaylist `
                -Index ([int]$Matches[1] - 1) `
                -Play
        }

        '^playlist delete (\d+)$' {

            Remove-LocalPlaylist (
                [int]$Matches[1] - 1
            )
        }

        '^playlist show$' {

            Show-Songs $script:CurrentPlaylist
        }

        '^search (.+)$' {

            Search-Command $Matches[1]
        }

        '^plays (\d+)$' {

            Play-SearchResult (
                [int]$Matches[1] - 1
            )
        }

        '^queues (\d+)$' {

            Add-SearchResultToQueue (
                [int]$Matches[1] - 1
            )
        }

        '^play (\d+)$' {

            Play-Song (
                [int]$Matches[1] - 1
            )
        }

        '^queue (\d+)$' {

            Add-ToQueue (
                [int]$Matches[1] - 1
            )
        }

        '^queue$' {

            Show-Queue
        }

        '^pause$' {

            Pause-Song
        }

        '^resume$' {

            Resume-Song
        }

        '^stop$' {

            Stop-Song
        }

        '^next$' {

            Next-Song
        }

        '^prev$' {

            Prev-Song
        }

        '^status$' {

            Show-Status
        }

        '^lyrics$' {

            Show-Lyrics
        }

        '^thumbnail$' {

            Show-Thumbnail
        }

        '^now$' {

            Show-NowPlayingArt
        }

        '^cache$' {

            Show-Cache
        }

        '^shuffle on$' {

            $script:Shuffle = $true
            Save-State

            Write-Host "Shuffle ON"
        }

        '^shuffle off$' {

            $script:Shuffle = $false
            Save-State

            Write-Host "Shuffle OFF"
        }

        '^autorec on$' {

            $script:AutoRecommend = $true
            Save-State

            Write-Host "YouTube Auto Recommendation ON"
        }

        '^autorec off$' {

            $script:AutoRecommend = $false
            Save-State

            Write-Host "YouTube Auto Recommendation OFF"
        }

            default {

                Write-Host ""
                Write-Host "Unknown command"
            }
        }

        Clean-Cache
    }
}
finally {
    $script:AutoAdvanceArmed = $false
    Stop-VLC
}

Write-Host ""
Write-Host "Bye..."
