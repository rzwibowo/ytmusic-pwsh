package main

import (
	"image"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigAcceptsUTF8BOM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{"httpPort":9595}`)...)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPPort != 9595 {
		t.Fatalf("HTTPPort = %d, want 9595", cfg.HTTPPort)
	}
}

func TestProfilePlaylistsURL(t *testing.T) {
	tests := map[string]string{
		"@openai":                          "https://www.youtube.com/@openai/playlists",
		"https://youtube.com/@openai":      "https://youtube.com/@openai/playlists",
		"https://youtube.com/@x/playlists": "https://youtube.com/@x/playlists",
		"https://example.com/@openai":      "",
	}
	for input, want := range tests {
		if got := profilePlaylistsURL(input); got != want {
			t.Errorf("profilePlaylistsURL(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSongFromYtEntryUsesChannelMetadata(t *testing.T) {
	got := songFromYtEntry(ytEntry{ID: "abc", Title: "A Song", Channel: "A Channel"})
	if got.Channel != "A Channel" {
		t.Fatalf("Channel = %q, want A Channel", got.Channel)
	}
}

func TestSongFromYtEntryFallsBackToUploader(t *testing.T) {
	got := songFromYtEntry(ytEntry{ID: "abc", Title: "A Song", Uploader: "An Uploader"})
	if got.Channel != "An Uploader" {
		t.Fatalf("Channel = %q, want An Uploader", got.Channel)
	}
}

func TestLyricsSearchTitle(t *testing.T) {
	got := lyricsSearchTitle("Artist - Song (Official Video) [Live Audio]")
	if got != "Artist - Song" {
		t.Fatalf("lyricsSearchTitle() = %q", got)
	}
}

func TestLyricsQueryUsesMetadataArtistAndTrack(t *testing.T) {
	metadata := &ytEntry{Title: "2112", Artist: "Reality Club", Track: "2112"}
	if got := lyricsQuery("2112", metadata); got != "Reality Club 2112" {
		t.Fatalf("lyricsQuery() = %q, want Reality Club 2112", got)
	}
}

func TestBestLyricsResultPrefersMatchingArtist(t *testing.T) {
	results := []lyricsResult{
		{TrackName: "2112", ArtistName: "Rush", PlainLyrics: "Rush lyrics"},
		{TrackName: "2112", ArtistName: "Reality Club", PlainLyrics: "Reality Club lyrics"},
	}
	got := bestLyricsResult("Reality Club - 2112", results)
	if got == nil || got.ArtistName != "Reality Club" {
		t.Fatalf("bestLyricsResult() = %#v, want Reality Club", got)
	}
}

func TestBestLyricsResultRejectsDifferentArtist(t *testing.T) {
	results := []lyricsResult{
		{TrackName: "2112", ArtistName: "Rush", PlainLyrics: "Rush lyrics"},
	}
	if got := bestLyricsResult("Reality Club - 2112", results); got != nil {
		t.Fatalf("bestLyricsResult() = %#v, want nil", got)
	}
}

func TestThumbnailRenderSizeUsesFixedRows(t *testing.T) {
	width, pixelHeight := thumbnailRenderSize(image.Rect(0, 0, 480, 360), 0, 2)
	if width != 8 || pixelHeight != 4 {
		t.Fatalf("thumbnailRenderSize() = %d, %d, want 8, 4", width, pixelHeight)
	}
}

func TestThumbnailRenderSizeKeepsConfiguredWidth(t *testing.T) {
	width, pixelHeight := thumbnailRenderSize(image.Rect(0, 0, 480, 360), 32, 0)
	if width != 32 || pixelHeight != 24 {
		t.Fatalf("thumbnailRenderSize() = %d, %d, want 32, 24", width, pixelHeight)
	}
}

func TestStatusLines(t *testing.T) {
	p := &player{autoRecommend: true, shuffle: true, currentSong: &Song{Title: "A Song"}}
	lines := p.statusLines(&vlcStatus{State: "playing", Position: .5, Time: 65, Length: 130}, 60)
	if lines[0] != "[> PLAYING] A Song" {
		t.Fatalf("title line = %q", lines[0])
	}
	if lines[1] != "[=======================o----------------------] 1:05 / 2:10" {
		t.Fatalf("seek line = %q", lines[1])
	}
	if lines[2] != "AUTO REC ON | SHUFFLE ON | Space Play/Pause | Left/Right ..." {
		t.Fatalf("shortcut line = %q", lines[2])
	}
}

func TestSafeID(t *testing.T) {
	if got := safeID("PL/a:b?c"); got != "PL_a_b_c" {
		t.Fatalf("safeID() = %q", got)
	}
}
