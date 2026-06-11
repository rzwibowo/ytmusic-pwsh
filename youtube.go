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

func (p *player) listing(target string, extra ...string) (*ytListing, error) {
	args := []string{"--js-runtimes", p.cfg.JSRuntime, "--flat-playlist"}
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

func (p *player) searchYouTube(keyword string) []Song {
	fmt.Println("\nSearching:", keyword)
	listing, err := p.listing("ytsearch20:" + keyword)
	if err != nil {
		fmt.Println("Search failed:", err)
		return nil
	}
	songs := make([]Song, 0, len(listing.Entries))
	for _, entry := range listing.Entries {
		if entry.ID != "" && entry.Title != "" {
			songs = append(songs, Song{ID: entry.ID, Title: entry.Title})
		}
	}
	if len(songs) == 0 {
		fmt.Println("No search results found")
	}
	return songs
}

func (p *player) youtubePlaylist(target string) (*PlaylistSource, error) {
	listing, err := p.listing(target)
	if err != nil {
		return nil, err
	}
	songs := make([]Song, 0, len(listing.Entries))
	for _, entry := range listing.Entries {
		if entry.ID != "" && entry.Title != "" {
			songs = append(songs, Song{ID: entry.ID, Title: entry.Title})
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
	listing, err := p.listing(target)
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
	listing, err := p.listing(target, "--playlist-end", "25")
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
			return &Song{ID: entry.ID, Title: entry.Title}
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
	cmd := exec.Command(exe, "--js-runtimes", p.cfg.JSRuntime, "--no-playlist", "-f", "ba", "-g", target)
	hideProcessWindow(cmd)
	var output, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &stderr
	if err := cmd.Start(); err != nil {
		fmt.Println("Failed to start yt-dlp:", err)
		return ""
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	spinner := []byte{'|', '/', '-', '\\'}
	fmt.Print("[STREAM] |")
	frame := 0
	for {
		select {
		case err := <-done:
			fmt.Print("\r[STREAM]   \n")
			if err != nil {
				fmt.Println(strings.TrimSpace(stderr.String()))
				return ""
			}
			for _, line := range strings.Split(output.String(), "\n") {
				if line = strings.TrimSpace(line); line != "" {
					return line
				}
			}
			return ""
		default:
			fmt.Printf("\r[STREAM] %c", spinner[frame%len(spinner)])
			frame++
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
	cmd := exec.Command(exe, "--js-runtimes", p.cfg.JSRuntime, "-f", "ba", "-o", output, target)
	hideProcessWindow(cmd)
	_ = cmd.Start()
}
