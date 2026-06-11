package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func readJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func (p *player) restore() {
	if err := readJSON(filepath.Join(p.cfg.DataDir, "playlist.json"), &p.playlist); err == nil && len(p.playlist) > 0 {
		fmt.Printf("Playlist restored: %d songs\n", len(p.playlist))
	}
	var state savedState
	if err := readJSON(filepath.Join(p.cfg.DataDir, "state.json"), &state); err == nil {
		p.shuffle = state.Shuffle
		p.autoRecommend = state.AutoRecommend
	}
	p.restoreLibrary()
}

func (p *player) savePlaylist() {
	if err := writeJSON(filepath.Join(p.cfg.DataDir, "playlist.json"), p.playlist); err != nil {
		fmt.Println("Could not save playlist:", err)
	}
}

func (p *player) saveState() {
	state := savedState{Shuffle: p.shuffle, AutoRecommend: p.autoRecommend, CurrentIndex: p.currentIndex}
	if err := writeJSON(filepath.Join(p.cfg.DataDir, "state.json"), state); err != nil {
		fmt.Println("Could not save player state:", err)
	}
}

func (p *player) restoreLibrary() {
	var library []PlaylistEntry
	if err := readJSON(filepath.Join(p.cfg.PlaylistLibraryDir, "library.json"), &library); err == nil {
		p.library = library
	} else {
		p.library = nil
	}
}

func (p *player) saveLibrary() {
	if err := writeJSON(filepath.Join(p.cfg.PlaylistLibraryDir, "library.json"), p.library); err != nil {
		fmt.Println("Could not save playlist library:", err)
	}
}

func (p *player) cacheFile(videoID string) string {
	entries, _ := os.ReadDir(p.cfg.CacheDir)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		base := strings.TrimSuffix(name, filepath.Ext(name))
		if base == videoID && ext != ".part" && ext != ".ytdl" && !strings.HasSuffix(name, ".thumbnail.jpg") {
			return filepath.Join(p.cfg.CacheDir, name)
		}
	}
	return ""
}

func (p *player) cacheSize() int64 {
	var total int64
	entries, _ := os.ReadDir(p.cfg.CacheDir)
	for _, entry := range entries {
		if info, err := entry.Info(); err == nil && !info.IsDir() {
			total += info.Size()
		}
	}
	return total
}

func (p *player) cleanCache() {
	limit := int64(p.cfg.CacheLimitMB) * 1024 * 1024
	for p.cacheSize() > limit {
		entries, _ := os.ReadDir(p.cfg.CacheDir)
		type candidate struct {
			path string
			info os.FileInfo
		}
		var files []candidate
		for _, entry := range entries {
			if info, err := entry.Info(); err == nil && !info.IsDir() {
				files = append(files, candidate{filepath.Join(p.cfg.CacheDir, entry.Name()), info})
			}
		}
		if len(files) == 0 {
			return
		}
		sort.Slice(files, func(i, j int) bool {
			return files[i].info.ModTime().Before(files[j].info.ModTime())
		})
		fmt.Println("Removing cache", files[0].info.Name())
		_ = os.Remove(files[0].path)
	}
}

func safeID(id string) string {
	var b strings.Builder
	for _, r := range id {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return b.String()
}
