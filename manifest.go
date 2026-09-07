package main

type manifestEntry struct {
	downloadCache
	Position int    `json:"index,omitempty"`
	FinalURL string `json:"final_url,omitempty"`
	Page     string `json:"page"`
	URL      string `json:"url,omitempty"`
	Kind     string `json:"kind"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256"`
}
