package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

type Config struct {
	CacheLimitMB       int    `json:"cacheLimitMB"`
	VLCPath            string `json:"vlcPath"`
	YtDlp              string `json:"ytDlp"`
	JSRuntime          string `json:"jsRuntime"`
	HTTPPort           int    `json:"httpPort"`
	HTTPPassword       string `json:"httpPassword"`
	DataDir            string `json:"dataDir"`
	CacheDir           string `json:"cacheDir"`
	PlaylistLibraryDir string `json:"playlistLibraryDir"`
	ThumbnailWidth     int    `json:"thumbnailWidth"`
	LyricsAPI          string `json:"lyricsApi"`
}

func defaultConfig() Config {
	return Config{
		CacheLimitMB:       500,
		VLCPath:            "vlc",
		YtDlp:              "yt-dlp",
		JSRuntime:          "node",
		HTTPPort:           9494,
		HTTPPassword:       "ytmusic",
		DataDir:            filepath.Join(".", "data"),
		CacheDir:           filepath.Join(".", "cache"),
		PlaylistLibraryDir: filepath.Join(".", "data", "playlists"),
		ThumbnailWidth:     32,
		LyricsAPI:          "https://lrclib.net/api/search",
	}
}

func loadConfig(path string) (Config, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if local := filepath.Join(".", "yt-dlp.exe"); fileExists(local) {
			cfg.YtDlp = local
		}
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func saveConfig(path string, cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func resolveExecutable(value string) (string, error) {
	if value == "" {
		return "", errors.New("empty executable path")
	}
	if filepath.IsAbs(value) || filepath.Dir(value) != "." {
		if fileExists(value) {
			return value, nil
		}
		return "", os.ErrNotExist
	}
	return exec.LookPath(value)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
