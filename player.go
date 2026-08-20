package main

import (
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (p *player) playSong(index int) {
	if index < 0 || index >= len(p.playlist) {
		fmt.Println("Invalid song number")
		return
	}
	song := p.playlist[index]
	p.currentSong = &p.playlist[index]
	p.currentIndex = index
	p.autoAdvanceArmed = false
	nowTitle := song.nowPlayingTitle()
	setConsoleTitle("▶️ " + nowTitle + " — ytplayer go")
	fmt.Printf("\nNow Playing:\n%s\n", nowTitle)
	cached := p.cacheFile(song.ID)
	if cached != "" {
		now := time.Now()
		_ = os.Chtimes(cached, now, now)
		fmt.Println("[CACHE]")
		if absolute, err := filepath.Abs(cached); err == nil {
			cached = absolute
		}
		if err := p.playStream(cached); err != nil {
			fmt.Println("VLC playback failed:", err)
			return
		}
	} else {
		stream := p.streamURL(song.ID, song.SourceURL)
		if stream == "" {
			fmt.Println("Failed to get stream")
			return
		}
		if err := p.playStream(stream); err != nil {
			fmt.Println("VLC playback failed:", err)
			return
		}
		if song.SourceURL == "" {
			p.backgroundCache(song.ID)
		}
	}
	p.preloadNext()
	p.saveState()
}

func (p *player) togglePlayback() {
	status, err := p.getVLCStatus()
	if err != nil {
		fmt.Println("VLC is not responding")
		return
	}
	switch status.State {
	case "playing":
		_ = p.vlcRequest("pl_pause", nil)
		title := p.currentTitle()
		setConsoleTitle("⏸️ " + title + " — ytplayer go")
		fmt.Println("Paused:", title)
	case "paused":
		_ = p.vlcRequest("pl_pause", nil)
		title := p.currentTitle()
		setConsoleTitle("▶️ " + title + " — ytplayer go")
		fmt.Println("Playing:", title)
	default:
		if p.currentIndex >= 0 {
			p.playSong(p.currentIndex)
		} else if len(p.playlist) > 0 {
			p.playSong(0)
		} else {
			fmt.Println("Playlist is empty")
		}
	}
}

func (p *player) nextIndex() int {
	if len(p.queue) > 0 {
		song := p.queue[0]
		p.queue = p.queue[1:]
		for i := range p.playlist {
			if p.playlist[i].ID == song.ID {
				return i
			}
		}
	}
	if p.shuffle {
		return rand.Intn(len(p.playlist))
	}
	if p.autoRecommend && p.currentIndex >= len(p.playlist)-1 && p.currentSong != nil && p.currentSong.SourceURL == "" {
		if recommendation := p.recommendation(p.currentSong.ID); recommendation != nil {
			p.playlist = append(p.playlist, *recommendation)
			fmt.Printf("Recommended:\n%s\n", recommendation.Title)
			return len(p.playlist) - 1
		}
	}
	return (p.currentIndex + 1) % len(p.playlist)
}

func (p *player) nextSong() {
	if len(p.playlist) > 0 {
		p.playSong(p.nextIndex())
	}
}

func (p *player) prevSong() {
	if len(p.playlist) == 0 {
		return
	}
	index := p.currentIndex - 1
	if index < 0 {
		index = len(p.playlist) - 1
	}
	p.playSong(index)
}

func (p *player) preloadNext() {
	next := p.currentIndex + 1
	if next < 0 || next >= len(p.playlist) || p.cacheFile(p.playlist[next].ID) != "" {
		return
	}
	fmt.Printf("\nPreloading:\n%s\n", p.playlist[next].Title)
	p.backgroundCache(p.playlist[next].ID)
}

func (p *player) autoNext(status *vlcStatus) bool {
	if p.currentSong == nil || status == nil {
		return false
	}
	if status.State == "playing" {
		if statusHasPlaybackProgress(status) {
			p.autoAdvanceArmed = true
		}
		return false
	}
	if status.State == "stopped" && p.autoAdvanceArmed {
		p.autoAdvanceArmed = false
		fmt.Println("\nSong finished. Playing next...")
		p.nextSong()
		return true
	}
	return false
}

func statusHasPlaybackProgress(status *vlcStatus) bool {
	return status.Length > 0 || status.Time > 0 || status.Position > 0
}

func (p *player) loadYouTubePlaylist(rawURL string) {
	rawURL = sanitizeInput(rawURL)
	u, err := url.Parse(rawURL)
	if err != nil || !u.IsAbs() || (u.Scheme != "http" && u.Scheme != "https") {
		fmt.Println("Invalid playlist URL")
		return
	}
	fmt.Println("\nLoading playlist...")
	stopSpinner := startSpinner("Loading playlist")
	source, err := p.youtubePlaylist(rawURL)
	stopSpinner()
	if err != nil {
		fmt.Println("Failed to load playlist:", err)
		return
	}
	p.playlist = source.Songs
	p.currentSource = source
	p.currentIndex, p.currentSong = -1, nil
	p.savePlaylist()
	fmt.Printf("\nPlaylist loaded:\n%d songs\n", len(p.playlist))
}

func (p *player) playSingleURL(rawURL string) {
	rawURL = sanitizeInput(rawURL)
	u, err := url.Parse(rawURL)
	if err != nil || !u.IsAbs() || (u.Scheme != "http" && u.Scheme != "https") {
		fmt.Println("Invalid media URL")
		return
	}
	song := Song{ID: u.String(), Title: u.String(), SourceURL: u.String()}
	p.playlist, p.currentSource, p.currentSong, p.currentIndex = []Song{song}, nil, nil, -1
	p.playSong(0)
}

func (p *player) saveLoadedPlaylist() {
	if p.currentSource == nil {
		fmt.Println("No URL playlist to save. Use playlist load <url> first.")
		return
	}
	id := p.currentSource.ID
	if id == "" {
		id = strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	fileName := safeID(id) + ".json"
	if err := writeJSON(filepath.Join(p.cfg.PlaylistLibraryDir, fileName), p.playlist); err != nil {
		fmt.Println("Could not save local playlist:", err)
		return
	}
	p.restoreLibrary()
	title := p.currentSource.Title
	if title == "" {
		title = "YouTube Playlist"
	}
	entry := PlaylistEntry{
		ID: id, Title: title, URL: p.currentSource.URL, File: fileName,
		SongCount: len(p.playlist), ImportedAt: time.Now().Format(time.RFC3339Nano),
	}
	updated := false
	for i := range p.library {
		if p.library[i].ID == id {
			p.library[i], updated = entry, true
			break
		}
	}
	if !updated {
		p.library = append(p.library, entry)
	}
	p.saveLibrary()
	if updated {
		fmt.Println("Updated local playlist:", title)
	} else {
		fmt.Println("Saved local playlist:", title)
	}
}

func (p *player) importProfilePlaylist(index int) bool {
	if index < 0 || index >= len(p.profileResults) {
		fmt.Println("Invalid profile playlist number")
		return false
	}
	item := p.profileResults[index]
	fmt.Printf("\nImporting playlist:\n%s\n", item.Title)
	stopSpinner := startSpinner("Importing playlist")
	source, err := p.youtubePlaylist(item.URL)
	stopSpinner()
	if err != nil {
		fmt.Println("Playlist was not added:", err)
		return false
	}
	fileName := safeID(item.ID) + ".json"
	if err := writeJSON(filepath.Join(p.cfg.PlaylistLibraryDir, fileName), source.Songs); err != nil {
		fmt.Println("Playlist was not added:", err)
		return false
	}
	p.restoreLibrary()
	entry := PlaylistEntry{
		ID: item.ID, Title: item.Title, URL: item.URL, File: fileName,
		SongCount: len(source.Songs), ImportedAt: time.Now().Format(time.RFC3339Nano),
	}
	updated := false
	for i := range p.library {
		if p.library[i].ID == item.ID {
			p.library[i], updated = entry, true
			break
		}
	}
	if !updated {
		p.library = append(p.library, entry)
	}
	p.saveLibrary()
	fmt.Printf("Added local playlist: %d songs\n", len(source.Songs))
	return true
}

func (p *player) useLocalPlaylist(index int, play bool) {
	p.restoreLibrary()
	if index < 0 || index >= len(p.library) {
		fmt.Println("Invalid local playlist number")
		return
	}
	item := p.library[index]
	var songs []Song
	if err := readJSON(filepath.Join(p.cfg.PlaylistLibraryDir, item.File), &songs); err != nil {
		fmt.Println("Could not read local playlist:", err)
		return
	}
	if len(songs) == 0 {
		fmt.Println("Local playlist is empty")
		return
	}
	p.playlist, p.currentSource, p.currentSong, p.currentIndex, p.queue = songs, nil, nil, -1, nil
	p.savePlaylist()
	p.saveState()
	fmt.Printf("\nSelected playlist:\n%s (%d songs)\n", item.Title, len(songs))
	if play {
		p.playSong(0)
	}
}

func (p *player) deleteLocalPlaylist(index int) {
	p.restoreLibrary()
	if index < 0 || index >= len(p.library) {
		fmt.Println("Invalid local playlist number")
		return
	}
	item := p.library[index]
	fmt.Printf("\nDelete '%s' from local library? (y/N): ", item.Title)
	confirmed := false
	if p.console != nil {
		for {
			key, ok := p.console.readKey(30 * time.Second)
			if !ok || key.virtual == keyEnter {
				break
			}
			if key.char == 'y' || key.char == 'Y' {
				fmt.Println("y")
				confirmed = true
				break
			}
			if key.char == 'n' || key.char == 'N' {
				fmt.Println("n")
				break
			}
		}
	}
	if !confirmed {
		fmt.Println("Delete cancelled")
		return
	}
	_ = os.Remove(filepath.Join(p.cfg.PlaylistLibraryDir, item.File))
	p.library = append(p.library[:index], p.library[index+1:]...)
	p.saveLibrary()
	fmt.Println("Deleted local playlist:", item.Title)
}

func (p *player) currentTitle() string {
	if p.currentSong == nil {
		return "Nothing playing"
	}
	return p.currentSong.nowPlayingTitle()
}

func (s Song) nowPlayingTitle() string {
	title := strings.TrimSpace(s.Track)
	if title == "" {
		title = strings.TrimSpace(s.Title)
	}
	artist := strings.TrimSpace(s.Artist)
	if artist == "" {
		return title
	}
	return fmt.Sprintf("%s - %s", title, artist)
}
