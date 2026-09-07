package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestMatchesCompletedFilesAndSkipsFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			fmt.Fprint(w, "<script src='/a.js'></script><script src='/a.js'></script><script src='/b.js'></script><script>inline()</script><script src='/missing.js'></script>")
		case "/a.js", "/b.js":
			fmt.Fprint(w, "identical content")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	manifest := filepath.Join(dir, "manifest.jsonl")
	code, out, err := invoke("-u", srv.URL, "--save", "--jsonl", "--manifest", manifest, "--output-dir", filepath.Join(dir, "scripts"))
	if code != 2 {
		t.Fatalf("%d %s", code, err)
	}
	items := decodeSources(t, out)
	if len(items) != 5 || items[0].SHA256 == "" || items[0].SHA256 != items[2].SHA256 || items[0].Path != items[1].Path {
		t.Fatalf("%#v", items)
	}
	f, e := os.Open(manifest)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	count := 0
	for {
		var entry manifestEntry
		if e := dec.Decode(&entry); e == io.EOF {
			break
		} else if e != nil {
			t.Fatal(e)
		}
		count++
		data, e := os.ReadFile(entry.Path)
		if e != nil {
			t.Fatal(e)
		}
		sum := sha256.Sum256(data)
		if entry.Size != int64(len(data)) || entry.SHA256 != fmt.Sprintf("%x", sum) || entry.Page != srv.URL {
			t.Fatalf("%#v", entry)
		}
	}
	if count != 3 {
		t.Fatalf("entries=%d, want three distinct completed files", count)
	}
}

func TestManifestValidationAndWriteErrors(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "input.html")
	html := "<script>inline()</script>"
	if err := os.WriteFile(file, []byte(html), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--manifest", filepath.Join(dir, "m.jsonl")},
		{"--html-file", file, "--base-url", "https://example.test", "--save", "--manifest", file},
		{"--html-file", file, "--base-url", "https://example.test", "--save", "--manifest", filepath.Join(dir, "m.jsonl"), "--output", filepath.Join(dir, "m.jsonl")},
	} {
		if code, _, err := invoke(args...); code != 3 {
			t.Fatalf("%v %d %s", args, code, err)
		}
	}
	data, _ := os.ReadFile(file)
	if string(data) != html {
		t.Fatal("input overwritten")
	}
	a := application{manifest: brokenWriter{}, manifestSeen: make(map[string]bool)}
	if err := a.emit(source{Page: "page", Path: "saved.js", SHA256: strings.Repeat("a", 64)}); err == nil || a.writeErr == nil {
		t.Fatal("manifest write failure lost")
	}
}
