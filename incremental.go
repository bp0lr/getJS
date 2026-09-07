package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type downloadCache struct {
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
	Context      string `json:"request_context,omitempty"`
}

func manifestKey(page, resource, kind string, position int) string {
	if kind == "inline" {
		return fmt.Sprintf("%s\x00inline\x00%d", page, position)
	}
	return page + "\x00" + resource
}

func readPrevious(c config) (map[string]manifestEntry, error) {
	entries := make(map[string]manifestEntry)
	if !c.incremental {
		return entries, nil
	}
	f, err := os.Open(c.manifest)
	if errors.Is(err, os.ErrNotExist) {
		return entries, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		var entry manifestEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return nil, fmt.Errorf("invalid previous manifest: %w", err)
		}
		entries[manifestKey(entry.Page, entry.URL, entry.Kind, entry.Position)] = entry
	}
	return entries, scanner.Err()
}

func (a *application) previousFile(s source) manifestEntry {
	return a.previous[manifestKey(s.Page, s.URL, s.Kind, s.Position)]
}

func (e manifestEntry) file() savedFile {
	return savedFile{Path: e.Path, Size: e.Size, SHA256: e.SHA256}
}

// A manifest path never grants access outside the current download root.
func verifiedFile(c config, file savedFile) bool {
	if file.Path == "" || file.Size < 0 || file.Size > c.maxBody || len(file.SHA256) != 64 {
		return false
	}
	base, err := filepath.Abs(c.outputDir)
	if err != nil {
		return false
	}
	abs, err := filepath.Abs(file.Path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(base, abs)
	if err != nil || !filepath.IsLocal(rel) {
		return false
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		return false
	}
	defer root.Close()
	info, err := root.Stat(rel)
	if err != nil || !info.Mode().IsRegular() || info.Size() != file.Size {
		return false
	}
	f, err := root.Open(rel)
	if err != nil {
		return false
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != file.Size {
		return false
	}
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(f, file.Size+1))
	return err == nil && n == file.Size && fmt.Sprintf("%x", hash.Sum(nil)) == file.SHA256
}

func requestContext(c config, origin *url.URL) string {
	// Include representation-affecting configuration without writing header values.
	data, _ := json.Marshal(struct {
		Version              int
		Origin, Proxy        string
		Headers              http.Header
		Insecure, SameOrigin bool
	}{1, canonicalURL(origin), c.proxy, c.headers, c.insecure, c.sameOriginOnly})
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func responseCache(resp *http.Response, fingerprint string) downloadCache {
	for _, value := range resp.Header.Values("Vary") {
		for _, token := range strings.Split(value, ",") {
			if strings.TrimSpace(token) == "*" {
				return downloadCache{}
			}
		}
	}
	for _, value := range resp.Header.Values("Cache-Control") {
		for _, token := range strings.Split(value, ",") {
			name, _, _ := strings.Cut(token, "=")
			if strings.EqualFold(strings.TrimSpace(name), "no-store") {
				return downloadCache{}
			}
		}
	}
	return downloadCache{ETag: resp.Header.Get("ETag"), LastModified: resp.Header.Get("Last-Modified"), Context: fingerprint}
}

type conditionalKey struct{}
type conditionalRequest struct {
	finalURL string
	cache    downloadCache
}

func applyConditional(req *http.Request) {
	if cached, ok := req.Context().Value(conditionalKey{}).(conditionalRequest); ok {
		req.Header.Del("If-None-Match")
		req.Header.Del("If-Modified-Since")
		if req.URL.String() == cached.finalURL {
			if cached.cache.ETag != "" {
				req.Header.Set("If-None-Match", cached.cache.ETag)
			} else if cached.cache.LastModified != "" {
				req.Header.Set("If-Modified-Since", cached.cache.LastModified)
			}
		}
	}
}

func withConditional(ctx context.Context, entry manifestEntry) context.Context {
	return context.WithValue(ctx, conditionalKey{}, conditionalRequest{entry.FinalURL, entry.downloadCache})
}
