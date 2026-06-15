package main

type Song struct {
	ID        string `json:"Id"`
	Title     string `json:"Title"`
	SourceURL string `json:"SourceUrl,omitempty"`
}

type PlaylistSource struct {
	ID    string
	Title string
	URL   string
	Songs []Song
}

type PlaylistEntry struct {
	ID         string `json:"Id"`
	Title      string `json:"Title"`
	URL        string `json:"Url"`
	File       string `json:"File"`
	SongCount  int    `json:"SongCount"`
	ImportedAt string `json:"ImportedAt"`
}

type savedState struct {
	Shuffle       bool `json:"Shuffle"`
	AutoRecommend bool `json:"AutoRecommend"`
	CurrentIndex  int  `json:"CurrentIndex"`
}

type vlcStatus struct {
	State    string  `json:"state"`
	Position float64 `json:"position"`
	Time     int     `json:"time"`
	Length   int     `json:"length"`
	Volume   int     `json:"volume"`
}

type ytEntry struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	Track       string `json:"track"`
	URL         string `json:"url"`
	WebpageURL  string `json:"webpage_url"`
	OriginalURL string `json:"original_url"`
}

type ytListing struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Entries []ytEntry `json:"entries"`
}

type lyricsResult struct {
	TrackName    string `json:"trackName"`
	ArtistName   string `json:"artistName"`
	PlainLyrics  string `json:"plainLyrics"`
	SyncedLyrics string `json:"syncedLyrics"`
}
