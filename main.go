package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type player struct {
	cfg              Config
	httpClient       *http.Client
	playlist         []Song
	queue            []Song
	currentIndex     int
	shuffle          bool
	autoRecommend    bool
	currentSong      *Song
	currentSource    *PlaylistSource
	profileResults   []PlaylistSource
	searchResults    []Song
	library          []PlaylistEntry
	vlcProcess       *exec.Cmd
	console          *consoleInput
	autoAdvanceArmed bool
}

func main() {
	configPath := flag.String("config", "config.json", "path to JSON configuration")
	httpPort := flag.Int("http-port", 0, "override VLC HTTP port")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not load config:", err)
		os.Exit(1)
	}
	if *httpPort > 0 && *httpPort <= 65535 {
		cfg.HTTPPort = *httpPort
	}
	if err := ensureDirectories(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "Could not create data directories:", err)
		os.Exit(1)
	}
	if !fileExists(*configPath) {
		if err := saveConfig(*configPath, cfg); err == nil {
			fmt.Println("Created", *configPath)
		}
	}

	p := &player{
		cfg:           cfg,
		httpClient:    &http.Client{Timeout: 10 * time.Second},
		currentIndex:  -1,
		autoRecommend: true,
	}
	if err := p.startVLC(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, "Run setup.bat or update config.json.")
		os.Exit(1)
	}
	defer p.stopVLC()
	p.cleanCache()
	p.restore()

	input, err := openConsoleInput()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Interactive terminal is required:", err)
		os.Exit(1)
	}
	defer input.close()
	p.console = input

	setConsoleTitle("ytplayer go")
	fmt.Printf("\n========================================\n ytplayer go\n========================================\nVLC HTTP port: %d\n", cfg.HTTPPort)
	showHelp()
	for {
		command := p.readCommand(input)
		if command == "" {
			continue
		}
		if !p.execute(command) {
			break
		}
		p.cleanCache()
	}
	fmt.Println("\nBye...")
}

func ensureDirectories(cfg Config) error {
	for _, path := range []string{cfg.DataDir, cfg.CacheDir, cfg.PlaylistLibraryDir} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (p *player) execute(command string) bool {
	command = expandCommand(command)
	lower := strings.ToLower(command)
	switch {
	case lower == "quit":
		return false
	case lower == "help":
		showHelp()
	case lower == "__playlist_up":
		p.showPlaylistWindow(true)
	case lower == "__playlist_down":
		p.showPlaylistWindow(false)
	case lower == "__toggle_autorecommend":
		p.autoRecommend = !p.autoRecommend
		p.saveState()
		fmt.Println("YouTube Auto Recommendation", onOff(p.autoRecommend))
	case lower == "__toggle_shuffle":
		p.shuffle = !p.shuffle
		p.saveState()
		fmt.Println("Shuffle", onOff(p.shuffle))
	case lower == "__toggle_playback":
		p.togglePlayback()
	case strings.HasPrefix(lower, "playlist load "):
		p.loadYouTubePlaylist(argumentAfter(command, "playlist load "))
	case lower == "playlist save":
		p.saveLoadedPlaylist()
	case lower == "playlist show":
		showSongs(p.playlist)
	case strings.HasPrefix(lower, "profile load "):
		p.profileResults = p.findProfilePlaylists(argumentAfter(command, "profile load "))
		p.showProfileResults()
	case lower == "profile show":
		p.showProfileResults()
	case lower == "profile add all":
		added := 0
		for i := range p.profileResults {
			fmt.Printf("\n[%d/%d]\n", i+1, len(p.profileResults))
			if p.importProfilePlaylist(i) {
				added++
			}
		}
		fmt.Printf("\nImport complete: %d of %d playlists added\n", added, len(p.profileResults))
	case strings.HasPrefix(lower, "profile add "):
		if index, ok := oneBasedIndex(argumentAfter(command, "profile add ")); ok {
			p.importProfilePlaylist(index)
		} else {
			fmt.Println("Invalid profile playlist number")
		}
	case lower == "playlist manager":
		p.showLibrary()
	case strings.HasPrefix(lower, "playlist use "):
		if index, ok := oneBasedIndex(argumentAfter(command, "playlist use ")); ok {
			p.useLocalPlaylist(index, false)
		}
	case strings.HasPrefix(lower, "playlist play "):
		if index, ok := oneBasedIndex(argumentAfter(command, "playlist play ")); ok {
			p.useLocalPlaylist(index, true)
		}
	case strings.HasPrefix(lower, "playlist delete "):
		if index, ok := oneBasedIndex(argumentAfter(command, "playlist delete ")); ok {
			p.deleteLocalPlaylist(index)
		}
	case strings.HasPrefix(lower, "search "):
		p.searchResults = p.searchYouTube(argumentAfter(command, "search "))
		showSongs(p.searchResults)
		if len(p.searchResults) > 0 {
			fmt.Println("\nUse: plays <number>\nOr : queues <number>\nOr : thumbs <number>")
		}
	case strings.HasPrefix(lower, "plays "):
		p.playSearchResult(argumentAfter(command, "plays "))
	case strings.HasPrefix(lower, "queues "):
		p.queueSearchResult(argumentAfter(command, "queues "))
	case strings.HasPrefix(lower, "thumbs "):
		p.showSearchThumbnail(argumentAfter(command, "thumbs "))
	case strings.HasPrefix(lower, "playurl "):
		p.playSingleURL(argumentAfter(command, "playurl "))
	case strings.HasPrefix(lower, "play "):
		if index, ok := oneBasedIndex(argumentAfter(command, "play ")); ok {
			p.playSong(index)
		}
	case lower == "queue":
		p.showQueue()
	case strings.HasPrefix(lower, "queue "):
		if index, ok := oneBasedIndex(argumentAfter(command, "queue ")); ok {
			p.addToQueue(index)
		}
	case lower == "pause":
		p.pause()
	case lower == "resume":
		p.resume()
	case lower == "stop":
		p.autoAdvanceArmed = false
		_ = p.vlcRequest("pl_stop", nil)
		setConsoleTitle("⏹️ ytplayer go")
		fmt.Println("Stopped")
	case lower == "next":
		p.nextSong()
	case lower == "prev":
		p.prevSong()
	case lower == "status":
		p.showStatus()
	case lower == "lyrics":
		p.showLyrics()
	case lower == "thumbnail":
		p.showThumbnail()
	case lower == "now":
		p.showThumbnail()
		p.showLyrics()
	case lower == "cache":
		p.showCache()
	case lower == "shuffle on":
		p.shuffle = true
		p.saveState()
		fmt.Println("Shuffle ON")
	case lower == "shuffle off":
		p.shuffle = false
		p.saveState()
		fmt.Println("Shuffle OFF")
	case lower == "autorec on":
		p.autoRecommend = true
		p.saveState()
		fmt.Println("YouTube Auto Recommendation ON")
	case lower == "autorec off":
		p.autoRecommend = false
		p.saveState()
		fmt.Println("YouTube Auto Recommendation OFF")
	default:
		fmt.Println("\nUnknown command")
	}
	return true
}

func (p *player) playSearchResult(raw string) {
	index, ok := oneBasedIndex(raw)
	if !ok || index >= len(p.searchResults) {
		fmt.Println("Invalid search result number")
		return
	}
	song := p.searchResults[index]
	for i := range p.playlist {
		if p.playlist[i].ID == song.ID {
			p.playSong(i)
			return
		}
	}
	p.playlist = append(p.playlist, song)
	p.playSong(len(p.playlist) - 1)
}

func (p *player) queueSearchResult(raw string) {
	index, ok := oneBasedIndex(raw)
	if !ok || index >= len(p.searchResults) {
		fmt.Println("Invalid search result number")
		return
	}
	song := p.searchResults[index]
	playlistIndex := -1
	for i := range p.playlist {
		if p.playlist[i].ID == song.ID {
			playlistIndex = i
			break
		}
	}
	if playlistIndex < 0 {
		p.playlist = append(p.playlist, song)
		playlistIndex = len(p.playlist) - 1
		p.savePlaylist()
	}
	p.addToQueue(playlistIndex)
}

func (p *player) addToQueue(index int) {
	if index < 0 || index >= len(p.playlist) {
		return
	}
	p.queue = append(p.queue, p.playlist[index])
	fmt.Printf("\nQueued:\n%s\n", p.playlist[index].Title)
}

func (p *player) showQueue() {
	if len(p.queue) == 0 {
		fmt.Println("Queue empty")
		return
	}
	fmt.Println()
	showSongs(p.queue)
}

func (p *player) pause() {
	status, err := p.getVLCStatus()
	if err != nil || status.State != "playing" {
		fmt.Println("Nothing is playing")
		return
	}
	_ = p.vlcRequest("pl_pause", nil)
	title := p.currentTitle()
	setConsoleTitle("⏸️ " + title + " — ytplayer go")
	fmt.Println("Paused:", title)
}

func (p *player) resume() {
	status, err := p.getVLCStatus()
	if err != nil || status.State != "paused" {
		fmt.Println("Nothing to resume")
		return
	}
	_ = p.vlcRequest("pl_pause", nil)
	title := p.currentTitle()
	setConsoleTitle("▶️ " + title + " — ytplayer go")
	fmt.Println("Playing:", title)
}

func (p *player) showStatus() {
	status, err := p.getVLCStatus()
	if err != nil {
		fmt.Println("No active playback")
		return
	}
	fmt.Printf("\nSong :\n%s\n\nState :\n%s\n\nVolume :\n%d\n\nPosition :\n%.1f %%\n",
		p.currentTitle(), status.State, status.Volume, status.Position*100)
}

func (p *player) showCache() {
	fmt.Printf("\nCache Usage:\n%.2f MB / %d MB\n\n", float64(p.cacheSize())/(1024*1024), p.cfg.CacheLimitMB)
	entries, _ := os.ReadDir(p.cfg.CacheDir)
	for _, entry := range entries {
		if info, err := entry.Info(); err == nil && !info.IsDir() {
			fmt.Printf("%-48s %8.2f MB\n", entry.Name(), float64(info.Size())/(1024*1024))
		}
	}
}

type commandAlias struct {
	alias   string
	command string
}

func commandAliases() []commandAlias {
	return []commandAlias{
		{"plld", "playlist load"},
		{"plsv", "playlist save"},
		{"plsh", "playlist show"},
		{"pfld", "profile load"},
		{"pfsh", "profile show"},
		{"pfad", "profile add"},
		{"plmg", "playlist manager"},
		{"plus", "playlist use"},
		{"plpy", "playlist play"},
		{"pldl", "playlist delete"},
		{"s", "search"},
		{"ps", "plays"},
		{"qs", "queues"},
		{"tbs", "thumbs"},
		{"pu", "playurl"},
		{"p", "play"},
		{"q", "queue"},
		{"ly", "lyrics"},
		{"tb", "thumbnail"},
	}
}

func baseCommands() []string {
	return []string{
		"playlist load",
		"playlist save",
		"playlist show",
		"profile load",
		"profile show",
		"profile add",
		"playlist manager",
		"playlist use",
		"playlist play",
		"playlist delete",
		"search",
		"plays",
		"queues",
		"thumbs",
		"playurl",
		"play",
		"queue",
		"pause",
		"resume",
		"stop",
		"next",
		"prev",
		"shuffle on",
		"shuffle off",
		"autorec on",
		"autorec off",
		"status",
		"lyrics",
		"thumbnail",
		"now",
		"cache",
		"help",
		"quit",
	}
}

func commandCompletions() []string {
	commands := append([]string{}, baseCommands()...)
	for _, item := range commandAliases() {
		commands = append(commands, item.alias)
	}
	return commands
}

func commonPrefix(left, right string) string {
	leftRunes := []rune(left)
	rightRunes := []rune(right)
	limit := minInt(len(leftRunes), len(rightRunes))
	for i := 0; i < limit; i++ {
		if leftRunes[i] != rightRunes[i] {
			return string(leftRunes[:i])
		}
	}
	return string(leftRunes[:limit])
}

func expandCommand(command string) string {
	command = sanitizeInput(command)
	lower := strings.ToLower(command)
	for _, item := range commandAliases() {
		if lower == item.alias {
			return item.command
		}
		if strings.HasPrefix(lower, item.alias+" ") {
			return item.command + command[len(item.alias):]
		}
	}
	return command
}

func sanitizeInput(value string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, value))
}

func oneBasedIndex(value string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	return n - 1, err == nil && n > 0
}

func argumentAfter(command, prefix string) string { return strings.TrimSpace(command[len(prefix):]) }
func onOff(value bool) string {
	if value {
		return "ON"
	}
	return "OFF"
}
func clamp(value, low, high int) int { return minInt(maxInt(value, low), high) }
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func decodeJSON(reader io.Reader, target any) error { return json.NewDecoder(reader).Decode(target) }
func stringPathSeparator() string                   { return string(filepath.Separator) }
func sleep100ms()                                   { time.Sleep(100 * time.Millisecond) }
