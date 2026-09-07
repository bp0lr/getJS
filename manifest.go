package main

type manifestEntry struct {
	Page   string `json:"page"`
	URL    string `json:"url,omitempty"`
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
