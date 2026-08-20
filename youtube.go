package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
)

func (p *player) runYtDlp(args ...string) ([]byte, error) {
	if hasCookiesFile() && !hasCookiesFileArg(args) {
		args = withCookiesFile(args)
	}
	output, err := p.runYtDlpOnce(args...)
	if err == nil || !isYtDlpLoginRequired(err.Error()) || hasCookiesFromBrowser(args) {
		return output, err
	}
	showFirefoxLoginHint()
	return p.runYtDlpOnce(withFirefoxCookies(args)...)
}

func (p *player) runYtDlpOnce(args ...string) ([]byte, error) {
	exe, err := resolveExecutable(p.cfg.YtDlp)
	if err != nil {
		return nil, fmt.Errorf("yt-dlp not found (%s)", p.cfg.YtDlp)
	}
	cmd := exec.Command(exe, args...)
	hideProcessWindow(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return nil, fmt.Errorf("%s", message)
		}
		return nil, err
	}
	return output, nil
}

func isYtDlpLoginRequired(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(lower, "sign in to confirm") ||
		strings.Contains(lower, "not a bot") ||
		strings.Contains(lower, "use --cookies-from-browser") ||
		strings.Contains(lower, "http error 403") ||
		strings.Contains(lower, "http error 429") ||
		strings.Contains(lower, "too many requests")
}

func hasCookiesFromBrowser(args []string) bool {
	for _, arg := range args {
		if arg == "--cookies-from-browser" || strings.HasPrefix(arg, "--cookies-from-browser=") {
			return true
		}
	}
	return false
}

func withFirefoxCookies(args []string) []string {
	copied := make([]string, 0, len(args)+2)
	copied = append(copied, "--cookies-from-browser", "firefox")
	copied = append(copied, args...)
	return copied
}

func showFirefoxLoginHint() {
	fmt.Println()
	fmt.Println(colorText(ansiYellow, "YouTube minta login / verifikasi bot."))
	fmt.Println("Login YouTube di Firefox, lalu coba lagi. yt-dlp akan pakai cookies Firefox.")
	fmt.Println("Jika Firefox masih terbuka dan gagal baca cookies, tutup Firefox dulu lalu ulangi.")
}

const cookiesFile = "cookies.txt"

func hasCookiesFile() bool {
	return fileExists(cookiesFile)
}

func hasCookiesFileArg(args []string) bool {
	for _, arg := range args {
		if arg == "--cookies" || strings.HasPrefix(arg, "--cookies=") {
			return true
		}
	}
	return false
}

func withCookiesFile(args []string) []string {
	copied := make([]string, 0, len(args)+2)
	copied = append(copied, "--cookies", cookiesFile)
	copied = append(copied, args...)
	return copied
}

func (p *player) listing(target string, extra ...string) (*ytListing, error) {
	args := []string{"--js-runtimes", p.cfg.JSRuntime, "--extractor-args", "youtube:player_client=web_embedded,android", "--flat-playlist"}
	args = append(args, extra...)
	args = append(args, "--dump-single-json", target)
	output, err := p.runYtDlp(args...)
	if err != nil {
		return nil, err
	}
	var listing ytListing
	if err := json.Unmarshal(output, &listing); err != nil {
		return nil, fmt.Errorf("yt-dlp returned invalid JSON: %w", err)
	}
	return &listing, nil
}

func (p *player) videoMetadata(videoID, sourceURL string) (*ytEntry, error) {
	target := sourceURL
	if target == "" {
		target = "https://www.youtube.com/watch?v=" + url.QueryEscape(videoID)
	}
	output, err := p.runYtDlp(
		"--js-runtimes", p.cfg.JSRuntime,
		"--extractor-args", "youtube:player_client=web_embedded,android",
		"--no-playlist",
		"--skip-download",
		"--dump-single-json",
		target,
	)
	if err != nil {
		return nil, err
	}
	var entry ytEntry
	if err := json.Unmarshal(output, &entry); err != nil {
		return nil, fmt.Errorf("yt-dlp returned invalid metadata: %w", err)
	}
	return &entry, nil
}

func (p *player) searchYouTube(keyword string) []Song {
	fmt.Println("\nSearching:", keyword)
	stopSpinner := startSpinner("Searching YouTube")
	listing, err := p.listing("ytsearch20:" + keyword)
	stopSpinner()
	if err != nil {
		fmt.Println("Search failed:", err)
		return nil
	}
	songs := make([]Song, 0, len(listing.Entries))
	for _, entry := range listing.Entries {
		if entry.ID != "" && entry.Title != "" {
			songs = append(songs, songFromYtEntry(entry))
		}
	}
	if len(songs) == 0 {
		fmt.Println("No search results found")
	}
	return songs
}

func songFromYtEntry(entry ytEntry) Song {
	channel := entry.Channel
	if channel == "" {
		channel = entry.Uploader
	}
	if channel == "" {
		channel = entry.Artist
	}
	return Song{ID: entry.ID, Title: entry.Title, Channel: channel, Artist: entry.Artist, Track: entry.Track}
}

func (p *player) youtubePlaylist(target string) (*PlaylistSource, error) {
	listing, err := p.listing(target)
	if err != nil {
		return nil, err
	}
	songs := make([]Song, 0, len(listing.Entries))
	for _, entry := range listing.Entries {
		if entry.ID != "" && entry.Title != "" {
			songs = append(songs, songFromYtEntry(entry))
		}
	}
	if len(songs) == 0 {
		return nil, fmt.Errorf("no valid playlist entries found")
	}
	return &PlaylistSource{ID: listing.ID, Title: listing.Title, URL: target, Songs: songs}, nil
}

func profilePlaylistsURL(profile string) string {
	profile = sanitizeInput(profile)
	if strings.HasPrefix(profile, "@") && len(profile) > 1 && !strings.ContainsAny(profile, "/?# ") {
		return "https://www.youtube.com/" + profile + "/playlists"
	}
	u, err := url.Parse(profile)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if host != "youtube.com" && !strings.HasSuffix(host, ".youtube.com") {
		return ""
	}
	u.RawQuery, u.Fragment = "", ""
	u.Path = strings.TrimSuffix(u.Path, "/")
	if !strings.HasSuffix(u.Path, "/playlists") {
		u.Path += "/playlists"
	}
	return u.String()
}

func (p *player) findProfilePlaylists(profile string) []PlaylistSource {
	target := profilePlaylistsURL(profile)
	if target == "" {
		fmt.Println("Invalid YouTube profile. Use @handle or a YouTube channel URL.")
		return nil
	}
	fmt.Println("\nLoading public playlists from:")
	fmt.Println(target)
	stopSpinner := startSpinner("Loading profile playlists")
	listing, err := p.listing(target)
	stopSpinner()
	if err != nil {
		fmt.Println("Failed to load profile playlists:", err)
		return nil
	}
	results := make([]PlaylistSource, 0, len(listing.Entries))
	for _, entry := range listing.Entries {
		if entry.ID == "" || entry.Title == "" {
			continue
		}
		entryURL := entry.WebpageURL
		if entryURL == "" {
			entryURL = entry.OriginalURL
		}
		if entryURL == "" {
			entryURL = entry.URL
		}
		if u, err := url.Parse(entryURL); err != nil || !u.IsAbs() {
			entryURL = "https://www.youtube.com/playlist?list=" + entry.ID
		}
		results = append(results, PlaylistSource{ID: entry.ID, Title: entry.Title, URL: entryURL})
	}
	return results
}

func (p *player) recommendation(videoID string) *Song {
	if videoID == "" {
		return nil
	}
	fmt.Println("\nLoading YouTube recommendations...")
	target := "https://www.youtube.com/watch?v=" + url.QueryEscape(videoID) + "&list=RD" + url.QueryEscape(videoID)
	stopSpinner := startSpinner("Loading YouTube recommendations")
	listing, err := p.listing(target, "--playlist-end", "25")
	stopSpinner()
	if err != nil {
		fmt.Println("Could not load YouTube recommendations:", err)
		return nil
	}
	known := make(map[string]bool, len(p.playlist))
	for _, song := range p.playlist {
		known[song.ID] = true
	}
	for _, entry := range listing.Entries {
		if entry.ID != "" && entry.Title != "" && entry.ID != videoID && !known[entry.ID] {
			song := songFromYtEntry(entry)
			return &song
		}
	}
	fmt.Println("No new YouTube recommendation found")
	return nil
}

func (p *player) streamURL(videoID, sourceURL string) string {
	target := sourceURL
	if target == "" {
		target = "https://www.youtube.com/watch?v=" + url.QueryEscape(videoID)
	}
	exe, err := resolveExecutable(p.cfg.YtDlp)
	if err != nil {
		fmt.Println("yt-dlp not found:", err)
		return ""
	}
	baseArgs := []string{"--js-runtimes", p.cfg.JSRuntime, "--extractor-args", "youtube:player_client=web_embedded,android", "--no-playlist", "-f", "ba/b", "-g", target}
	if hasCookiesFile() && !hasCookiesFileArg(baseArgs) {
		baseArgs = withCookiesFile(baseArgs)
	}
	stream, errMessage := p.streamURLWithArgs(exe, baseArgs...)
	if stream != "" || !isYtDlpLoginRequired(errMessage) {
		return stream
	}
	showFirefoxLoginHint()
	stream, _ = p.streamURLWithArgs(exe, withFirefoxCookies(baseArgs)...)
	return stream
}

func (p *player) streamURLWithArgs(exe string, args ...string) (string, string) {
	cmd := exec.Command(exe, args...)
	hideProcessWindow(cmd)
	var output, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &stderr
	if err := cmd.Start(); err != nil {
		fmt.Println("Failed to start yt-dlp:", err)
		return "", err.Error()
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	stopSpinner := startSpinner("[STREAM]")
	for {
		select {
		case err := <-done:
			stopSpinner()
			if err != nil {
				message := strings.TrimSpace(stderr.String())
				fmt.Println(message)
				return "", message
			}
			for _, line := range strings.Split(output.String(), "\n") {
				if line = strings.TrimSpace(line); line != "" {
					return line, ""
				}
			}
			return "", ""
		default:
			sleep100ms()
		}
	}
}

func (p *player) backgroundCache(videoID string) {
	if p.cacheFile(videoID) != "" {
		return
	}
	exe, err := resolveExecutable(p.cfg.YtDlp)
	if err != nil {
		return
	}
	target := "https://www.youtube.com/watch?v=" + url.QueryEscape(videoID)
	output := p.cfg.CacheDir + stringPathSeparator() + videoID + ".%(ext)s"
	cacheArgs := []string{"--js-runtimes", p.cfg.JSRuntime, "--extractor-args", "youtube:player_client=web_embedded,android", "-f", "ba/b", "-o", output, target}
	if hasCookiesFile() {
		cacheArgs = withCookiesFile(cacheArgs)
	}
	cmd := exec.Command(exe, cacheArgs...)
	hideProcessWindow(cmd)
	_ = cmd.Start()
}
