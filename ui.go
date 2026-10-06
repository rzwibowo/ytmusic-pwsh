package main

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	keyBackspace = 0x08
	keyEnter     = 0x0D
	keySpace     = 0x20
	keyLeft      = 0x25
	keyUp        = 0x26
	keyRight     = 0x27
	keyDown      = 0x28
	keyTab       = 0x09
	keyF1        = 0x70
	keyF5        = 0x74
	keyF6        = 0x75
	keyF7        = 0x76
	keyF8        = 0x77
	keyF9        = 0x78
)

const (
	ansiReset      = "\x1b[0m"
	ansiRed        = "\x1b[31m"
	ansiGreen      = "\x1b[32m"
	ansiYellow     = "\x1b[33m"
	ansiCyan       = "\x1b[36m"
	ansiDarkGray   = "\x1b[90m"
	ansiLightGreen = "\x1b[92m"
)

type keyPress struct {
	virtual uint16
	char    rune
	ctrl    bool
}

func colorText(color, text string) string {
	return color + text + ansiReset
}

func startSpinner(label string) func() {
	done := make(chan struct{})
	stopped := make(chan struct{})
	var once sync.Once
	frames := []rune{'|', '/', '-', '\\'}
	fmt.Printf("%s %c", label, frames[0])
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(120 * time.Millisecond)
		defer ticker.Stop()
		frame := 1
		for {
			select {
			case <-done:
				fmt.Print("\r" + strings.Repeat(" ", len(label)+2) + "\r")
				return
			case <-ticker.C:
				fmt.Printf("\r%s %c", label, frames[frame%len(frames)])
				frame++
			}
		}
	}()
	return func() {
		once.Do(func() {
			close(done)
			<-stopped
		})
	}
}

func formatPlaybackTime(seconds int) string {
	if seconds < 0 {
		seconds = 0
	}
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	secs := seconds % 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, secs)
	}
	return fmt.Sprintf("%d:%02d", minutes, secs)
}

func (p *player) statusLines(status *vlcStatus, width int) []string {
	state, icon, position, elapsed, total := "VLC OFFLINE", "⏹️", 0, 0, 0
	if status != nil {
		state = strings.ToUpper(status.State)
		position = clamp(int(status.Position*100+0.5), 0, 100)
		elapsed, total = maxInt(status.Time, 0), maxInt(status.Length, 0)
		switch state {
		case "PLAYING":
			icon = "▶️"
		case "PAUSED":
			icon = "⏸️"
		case "STOPPED":
			icon = "⏹️"
		}
	}
	title := p.currentTitle()
	firstLine := fmt.Sprintf("%s %s | %s", icon, state, title)

	togglesLine := fmt.Sprintf(
		"F6 Lyrics: %s | F7 Shuffle: %s | F8 AutoRec: %s | F9 Thumbnail: %s",
		onOffCircle(p.showSyncedLyrics),
		onOffCircle(p.shuffle),
		onOffCircle(p.autoRecommend),
		onOffCircle(p.autoThumbnail),
	)
	keysLine := "Space Play/Pause | Left/Right Skip | F5/Up/Down List | F1 Help"
	
	lines := []string{
		truncateLine(firstLine, width),
		playbackProgressLine(position, elapsed, total, width, false),
	}
	
	if p.showSyncedLyrics {
		prevText := ""
		currText := "No synced lyrics available"
		nextText := ""
		
		if len(p.currentLyrics) > 0 && status != nil {
			currIdx := -1
			for i, line := range p.currentLyrics {
				if elapsed >= line.TimeSeconds {
					currIdx = i
				} else {
					break
				}
			}
			
			currText = "..."
			if currIdx >= 0 {
				currText = p.currentLyrics[currIdx].Text
				if currIdx > 0 {
					prevText = p.currentLyrics[currIdx-1].Text
				}
			}
			if currIdx+1 < len(p.currentLyrics) {
				nextText = p.currentLyrics[currIdx+1].Text
			}
			if currText == "" {
				currText = "..."
			}
		}
		
		lines = append(lines, truncateLine("   "+prevText, width))
		lines = append(lines, truncateLine(" 🎵 "+currText, width))
		lines = append(lines, truncateLine("   "+nextText, width))
	}
	
	lines = append(lines, truncateLine(togglesLine, width))
	lines = append(lines, truncateLine(keysLine, width))
	return lines
}

func onOffCircle(v bool) string {
	if v {
		return "🟢"
	}
	return "⚫"
}

func playbackProgressLine(position, elapsed, total, width int, colored bool) string {
	elapsedTime := formatPlaybackTime(elapsed)
	totalTime := formatPlaybackTime(total)
	timeText := fmt.Sprintf(" %s / %s", elapsedTime, totalTime)
	barWidth := maxInt(5, width-len([]rune(timeText))-2)
	if !colored {
		return truncateLine("["+seekBar(position, barWidth)+"]"+timeText, width)
	}
	return coloredSeekBar(position, barWidth) + ansiDarkGray + timeText + ansiReset
}

func seekBar(position, width int) string {
	if width <= 0 {
		return ""
	}
	if position >= 100 {
		return strings.Repeat("\u2593", width)
	}
	filled := position * width / 100
	return strings.Repeat("\u2593", filled) + "\u2588" + strings.Repeat("\u2591", width-filled-1)
}

func coloredSeekBar(position, width int) string {
	if width <= 0 {
		return ""
	}
	if position >= 100 {
		return ansiDarkGray + "[" + ansiGreen + strings.Repeat("\u2593", width) + ansiDarkGray + "]"
	}
	filled := position * width / 100
	return ansiDarkGray + "[" +
		ansiGreen + strings.Repeat("\u2588", filled) +
		ansiLightGreen + "\u2588" +
		ansiDarkGray + strings.Repeat("\u2591", maxInt(0, width-filled-1)) +
		ansiDarkGray + "]"
}

func truncateLine(line string, width int) string {
	runes := []rune(line)
	if len(runes) <= width {
		return line
	}
	if width <= 3 {
		return string(runes[:maxInt(0, width)])
	}
	return string(runes[:width-3]) + "..."
}

func (p *player) writeStatus(status *vlcStatus) {
	width := maxInt(20, terminalWidth()-1)
	lines := p.statusLines(status, width)
	position, elapsed, total := 0, 0, 0
	stateColor := ansiDarkGray
	if status == nil {
		stateColor = ansiRed
	} else {
		position = clamp(int(status.Position*100+0.5), 0, 100)
		elapsed, total = maxInt(status.Time, 0), maxInt(status.Length, 0)
		switch strings.ToLower(status.State) {
		case "playing":
			stateColor = ansiGreen
		case "paused":
			stateColor = ansiYellow
		case "stopped":
			stateColor = ansiDarkGray
		}
	}
	
	linesCount := len(lines)
	if linesCount > 0 {
		fmt.Printf("\x1b7\x1b[%dA\r", linesCount)
	}
	
	for i, line := range lines {
		if i > 0 {
			fmt.Print("\n")
		}
		if i == 0 {
			writeStatusLine(stateColor, line)
		} else if i == 1 {
			writeStatusLine("", playbackProgressLine(position, elapsed, total, width, true))
		} else if p.showSyncedLyrics && i >= 2 && i <= 4 {
			if i == 3 {
				writeStatusLine(ansiYellow, line)
			} else {
				writeStatusLine(ansiDarkGray, line)
			}
		} else {
			writeStatusLine(ansiCyan, line)
		}
	}
	fmt.Print("\x1b8")
}

func writeStatusLine(color, line string) {
	fmt.Printf("\x1b[2K\r%s%s%s", color, line, ansiReset)
}

func (p *player) expectedStatusLinesCount() int {
	if p.showSyncedLyrics {
		return 7
	}
	return 4
}

func (p *player) clearInteractiveBlock() {
	lines := p.expectedStatusLinesCount()
	fmt.Print("\r\x1b[2K")
	for i := 0; i < lines; i++ {
		fmt.Print("\x1b[1A\x1b[2K")
	}
	fmt.Print("\r")
}

func (p *player) submitCommand(buffer []rune) string {
	command := sanitizeInput(string(buffer))
	p.clearInteractiveBlock()
	return command
}

func autocompleteCommand(buffer []rune) ([]rune, bool) {
	current := string(buffer)
	sanitized := sanitizeInput(current)
	if sanitized == "" || strings.HasSuffix(current, " ") {
		return buffer, false
	}

	matches := make([]string, 0)
	lower := strings.ToLower(sanitized)
	for _, command := range commandCompletions() {
		if strings.HasPrefix(command, lower) {
			matches = append(matches, command)
		}
	}
	if len(matches) == 0 {
		return buffer, false
	}

	completed := matches[0]
	for _, match := range matches[1:] {
		completed = commonPrefix(completed, match)
	}
	if completed == lower {
		return buffer, false
	}
	return []rune(completed), true
}

func redrawPrompt(buffer []rune) {
	fmt.Print("\r\x1b[2K", colorText(ansiCyan, "ytplayer: "), string(buffer))
}

func (p *player) readCommand(input *consoleInput) string {
	lines := p.expectedStatusLinesCount()
	fmt.Print(strings.Repeat("\n", lines), colorText(ansiCyan, "ytplayer: "))
	var buffer []rune
	lastStatus := time.Time{}
	refresh := 750 * time.Millisecond
	for {
		select {
		case command := <-p.mediaCommands:
			return p.submitCommand([]rune(command))
		default:
		}
		if key, ok := input.readKey(100 * time.Millisecond); ok {
			switch key.virtual {
			case keyEnter:
				return p.submitCommand(buffer)
			case keyBackspace:
				if len(buffer) > 0 {
					buffer = buffer[:len(buffer)-1]
					fmt.Print("\b \b")
				}
				continue
			case keyTab:
				if completed, ok := autocompleteCommand(buffer); ok {
					buffer = completed
					redrawPrompt(buffer)
				}
				continue
			}
			if len(buffer) == 0 {
				switch key.virtual {
				case keyF1:
					return p.submitCommand([]rune("help"))
				case keyF5:
					return p.submitCommand([]rune("playlist show"))
				case keyF6:
					return p.submitCommand([]rune("__toggle_lyrics"))
				case keyF7:
					return p.submitCommand([]rune("__toggle_shuffle"))
				case keyF8:
					return p.submitCommand([]rune("__toggle_autorecommend"))
				case keyF9:
					return p.submitCommand([]rune("__toggle_autothumbnail"))
				case keySpace:
					return p.submitCommand([]rune("__toggle_playback"))
				case keyLeft:
					if key.ctrl {
						return p.submitCommand([]rune("__seek_backward"))
					}
					return p.submitCommand([]rune("prev"))
				case keyRight:
					if key.ctrl {
						return p.submitCommand([]rune("__seek_forward"))
					}
					return p.submitCommand([]rune("next"))
				case keyUp:
					return p.submitCommand([]rune("__playlist_up"))
				case keyDown:
					return p.submitCommand([]rune("__playlist_down"))
				}
			}
			if key.char != 0 && !unicode.IsControl(key.char) {
				buffer = append(buffer, key.char)
				fmt.Printf("%c", key.char)
			}
		}
		if time.Since(lastStatus) >= refresh {
			lastStatus = time.Now()
			status, _ := p.getVLCStatus()
			if status != nil && status.State == "playing" {
				refresh = 750 * time.Millisecond
			} else {
				refresh = 2500 * time.Millisecond
			}
			if len(buffer) == 0 {
				p.writeStatus(status)
			}
			if p.autoNext(status) {
				fmt.Print(colorText(ansiCyan, "ytplayer: "), string(buffer))
			}
		}
	}
}

func showSongs(songs []Song) {
	for i, song := range songs {
		if song.Channel != "" {
			fmt.Printf("%3d. %s - %s\n", i+1, song.Title, song.Channel)
			continue
		}
		fmt.Printf("%3d. %s\n", i+1, song.Title)
	}
}

func (p *player) showFullPlaylist() {
	if len(p.playlist) == 0 {
		fmt.Println("Playlist is empty")
		return
	}
	fmt.Printf("\n%s\n", colorText(ansiYellow, fmt.Sprintf("Playlist (%d songs):", len(p.playlist))))
	for i, song := range p.playlist {
		marker := " "
		title := song.Title
		if song.Channel != "" {
			title = song.Title + " - " + song.Channel
		}
		if i == p.currentIndex {
			fmt.Printf("%s %3d. %s\n", colorText(ansiGreen, ">"), i+1, colorText(ansiGreen, title))
		} else {
			fmt.Printf("%s %3d. %s\n", marker, i+1, title)
		}
	}
}

func (p *player) showPlaylistWindow(up bool) {
	if len(p.playlist) == 0 {
		fmt.Println("Playlist is empty")
		return
	}
	anchor := maxInt(p.currentIndex, 0)
	start := anchor
	if up {
		start = maxInt(0, anchor-14)
	}
	end := minInt(len(p.playlist), start+15)
	fmt.Printf("\n%s\n", colorText(ansiYellow,
		fmt.Sprintf("Playlist (%d-%d of %d):", start+1, end, len(p.playlist))))
	for i := start; i < end; i++ {
		marker := " "
		if i == p.currentIndex {
			fmt.Printf("%s %3d. %s\n",
				colorText(ansiGreen, ">"), i+1, colorText(ansiGreen, p.playlist[i].Title))
			continue
		}
		fmt.Printf("%s %3d. %s\n", marker, i+1, p.playlist[i].Title)
	}
}

func (p *player) showLibrary() {
	p.restoreLibrary()
	fmt.Println("\n" + colorText(ansiYellow, "Local Playlist Manager:"))
	if len(p.library) == 0 {
		fmt.Println("No local playlists. Use playlist save or profile add <number|all> first.")
		return
	}
	for i, item := range p.library {
		fmt.Printf("%3d. %s (%d songs)\n", i+1, item.Title, item.SongCount)
	}
	fmt.Println("\n" + colorText(ansiYellow, "Use: playlist play <number>"))
	fmt.Println(colorText(ansiYellow, "Or : playlist use <number> (load without playing)"))
}

func (p *player) showProfileResults() {
	fmt.Println("\n" + colorText(ansiYellow, "Profile Public Playlists:"))
	if len(p.profileResults) == 0 {
		fmt.Println("No profile playlist results")
		return
	}
	for i, item := range p.profileResults {
		fmt.Printf("%3d. %s\n", i+1, item.Title)
	}
	fmt.Println("\n" + colorText(ansiYellow, "Use: profile add <number>"))
	fmt.Println(colorText(ansiYellow, "Or : profile add all"))
}

func showHelp() {
	fmt.Println()
	fmt.Println(colorText(ansiYellow, "Commands:"))
	fmt.Print(`
  playlist load <url>  (plld) Load YouTube playlist
  playlist save        (plsv) Save loaded URL playlist to local library
  playlist show        (plsh) Show loaded playlist
  profile load <url>   (pfld) Find public playlists from a profile
  profile show         (pfsh) Show profile playlist results
  profile add <n|all>  (pfad) Add profile playlist(s) to local library
  playlist manager     (plmg) Show local playlist library
  playlist use <n>     (plus) Select a local playlist
  playlist play <n>    (plpy) Select and play a local playlist
  playlist delete <n>  (pldl) Delete a local playlist
  search <keyword>     (s) Search YouTube
  searchplay <keyword> (sip) Search and immediately play first result
  searchqueue <kw>     (siq) Search and immediately queue first result
  plays <number>       (ps) Play a search result
  queues <number>      (qs) Queue a search result
  thumbs <number>      (tbs) Show a search result thumbnail
  playurl <url>        (pu) Play a direct media/YouTube URL
  play <number>        (p) Play a playlist song
  queue <number>       (q) Add playlist song to queue
  queue                (q) Show queue
  pause | resume       Pause or resume playback
  stop | next | prev   Playback navigation
  shuffle on|off       Toggle shuffle
  autorec on|off       Toggle YouTube recommendations
  status               Show playback status
  lyrics               (ly) Show lyrics
  thumbnail            (tb) Show color thumbnail
  now                  Show thumbnail and lyrics
  cache                Show cache usage
  help                 Show this help
  quit                 Exit player
`)
	fmt.Println()
	fmt.Println(colorText(ansiYellow, "Keys:"))
	fmt.Print(`
  Space  Play/Pause
  Left   Previous song (Ctrl+Left: Rewind 10s)
  Right  Next song (Ctrl+Right: Forward 10s)
  Up     Show previous playlist items
  Down   Show next playlist items
  F1     Show help
  F6     Toggle Interactive Lyrics
  F7     Toggle Shuffle
  F8     Toggle Auto Recommendation
  F9     Toggle Auto Thumbnail

System media keys (work even when window is not focused):
  Play/Pause, Stop, Next Track, Previous Track
`)
}
