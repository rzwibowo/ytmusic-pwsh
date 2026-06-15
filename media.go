package main

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func (p *player) currentSongOrWarn() *Song {
	if p.currentSong == nil {
		fmt.Println("Nothing is playing")
		return nil
	}
	return p.currentSong
}

func (p *player) thumbnailFile(videoID string) string {
	path := filepath.Join(p.cfg.CacheDir, videoID+".thumbnail.jpg")
	if fileExists(path) {
		return path
	}
	resp, err := p.httpClient.Get("https://i.ytimg.com/vi/" + url.PathEscape(videoID) + "/hqdefault.jpg")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	file, err := os.Create(path)
	if err != nil {
		return ""
	}
	_, copyErr := io.Copy(file, resp.Body)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(path)
		return ""
	}
	return path
}

func (p *player) showThumbnail() {
	song := p.currentSongOrWarn()
	if song == nil {
		return
	}
	path := p.thumbnailFile(song.ID)
	if path == "" {
		fmt.Println("Could not load thumbnail")
		return
	}
	file, err := os.Open(path)
	if err != nil {
		fmt.Println("Could not load thumbnail:", err)
		return
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	if err != nil {
		fmt.Println("Could not render color thumbnail:", err)
		return
	}
	width := clamp(p.cfg.ThumbnailWidth, 8, maxInt(8, terminalWidth()-2))
	bounds := img.Bounds()
	pixelHeight := maxInt(2, int(math.Round(float64(bounds.Dy())/float64(bounds.Dx())*float64(width))))
	if pixelHeight%2 != 0 {
		pixelHeight++
	}
	fmt.Println("\nThumbnail:")
	for y := 0; y < pixelHeight; y += 2 {
		for x := 0; x < width; x++ {
			top := sampleImage(img, x, y, width, pixelHeight)
			bottom := sampleImage(img, x, y+1, width, pixelHeight)
			tr, tg, tb, _ := top.RGBA()
			br, bg, bb, _ := bottom.RGBA()
			fmt.Printf("\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀",
				tr>>8, tg>>8, tb>>8, br>>8, bg>>8, bb>>8)
		}
		fmt.Print("\x1b[0m\n")
	}
}

func sampleImage(img image.Image, x, y, width, height int) interface {
	RGBA() (uint32, uint32, uint32, uint32)
} {
	b := img.Bounds()
	srcX := b.Min.X + x*b.Dx()/width
	srcY := b.Min.Y + y*b.Dy()/height
	return img.At(srcX, srcY)
}

var (
	titleNoise = regexp.MustCompile(`(?i)\s*[\(\[][^(\[]*(official|audio|video|lyrics?|visuali[sz]er|live)[^)\]]*[\)\]]`)
	timeTag    = regexp.MustCompile(`(?m)^\[(?:\d{2}:)?\d{2}:\d{2}(?:\.\d+)?\]\s*`)
	wordNoise  = regexp.MustCompile(`[^\p{L}\p{N}]+`)
	spaceRun   = regexp.MustCompile(`\s+`)
)

func lyricsSearchTitle(title string) string {
	return strings.TrimSpace(spaceRun.ReplaceAllString(titleNoise.ReplaceAllString(title, ""), " "))
}

func lyricsQuery(title string, metadata *ytEntry) string {
	if metadata != nil && metadata.Artist != "" {
		track := metadata.Track
		if track == "" {
			track = metadata.Title
		}
		if track != "" {
			return lyricsSearchTitle(metadata.Artist + " " + track)
		}
	}
	return lyricsSearchTitle(title)
}

func lyricsMatchScore(query string, result lyricsResult) float64 {
	queryWords := strings.Fields(wordNoise.ReplaceAllString(strings.ToLower(query), " "))
	if len(queryWords) == 0 {
		return 0
	}
	resultWords := strings.Fields(wordNoise.ReplaceAllString(
		strings.ToLower(result.ArtistName+" "+result.TrackName), " "))
	available := make(map[string]int, len(resultWords))
	for _, word := range resultWords {
		available[word]++
	}
	matches := 0
	for _, word := range queryWords {
		if available[word] > 0 {
			matches++
			available[word]--
		}
	}
	return float64(matches) / float64(len(queryWords))
}

func bestLyricsResult(query string, results []lyricsResult) *lyricsResult {
	bestIndex, bestScore := -1, 0.0
	for i := range results {
		if results[i].PlainLyrics == "" && results[i].SyncedLyrics == "" {
			continue
		}
		score := lyricsMatchScore(query, results[i])
		if score > bestScore {
			bestIndex, bestScore = i, score
		}
	}
	if bestIndex < 0 || bestScore < 0.6 {
		return nil
	}
	return &results[bestIndex]
}

func (p *player) showLyrics() {
	song := p.currentSongOrWarn()
	if song == nil {
		return
	}
	metadata, err := p.videoMetadata(song.ID, song.SourceURL)
	if err != nil {
		metadata = nil
	}
	query := lyricsQuery(song.Title, metadata)
	fmt.Printf("\nLyrics: %s\nSearching LRCLIB...\n", query)
	req, _ := http.NewRequest(http.MethodGet, p.cfg.LyricsAPI+"?q="+url.QueryEscape(query), nil)
	req.Header.Set("User-Agent", "ytmusic-cli-go/1.0")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		fmt.Println("Could not load lyrics:", err)
		return
	}
	defer resp.Body.Close()
	var results []lyricsResult
	if err := decodeJSON(resp.Body, &results); err != nil {
		fmt.Println("Could not load lyrics:", err)
		return
	}
	result := bestLyricsResult(query, results)
	if result == nil {
		fmt.Println("Lyrics not found")
		return
	}
	lyrics := result.PlainLyrics
	if lyrics == "" {
		lyrics = timeTag.ReplaceAllString(result.SyncedLyrics, "")
	}
	fmt.Printf("\n%s - %s\n\n%s\n\nLyrics provided by LRCLIB\n",
		result.TrackName, result.ArtistName, strings.TrimSpace(lyrics))
}
