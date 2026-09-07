package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestOfflineHTMLNeverRequestsScripts(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer srv.Close()
	dir := t.TempDir()
	file := filepath.Join(dir, "page.html")
	if err := os.WriteFile(file, []byte("<base href='/assets/'><script src='a.js'></script><link rel='modulepreload' href='b.js'><script>inline()</script>"), 0600); err != nil {
		t.Fatal(err)
	}
	code, out, err := invoke("--html-file", file, "--base-url", srv.URL+"/nested/page", "--jsonl", "--save", "--output-dir", filepath.Join(dir, "saved"))
	if code != 0 || calls.Load() != 0 {
		t.Fatalf("%d %s calls=%d", code, err, calls.Load())
	}
	items := decodeSources(t, out)
	if len(items) != 3 || items[0].URL != srv.URL+"/assets/a.js" || items[0].Status != 0 || items[0].Path != "" || !strings.HasPrefix(items[0].Page, "file:///") || items[2].Path == "" {
		t.Fatalf("%#v", items)
	}
	// Offline mode does not read a URL pipeline, even if it would fail to read.
	if code := run(context.Background(), []string{"--html-file", file, "--base-url", srv.URL}, failingReader{}, io.Discard, io.Discard); code != 0 {
		t.Fatalf("read ignored stdin: %d", code)
	}
}

func TestOfflineArgumentValidationAndInputPreservation(t *testing.T) {
	file := filepath.Join(t.TempDir(), "page.html")
	const html = "<script src='/a.js'></script>"
	if err := os.WriteFile(file, []byte(html), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--html-file", file},
		{"--base-url", "https://example.test"},
		{"--html-file", file, "--base-url", "https://example.test", "--resolve=true"},
		{"--html-file", file, "--base-url", "https://example.test", "--url", "https://example.test"},
		{"--html-file", file, "--base-url", "https://example.test", "--output", file},
	} {
		if code, _, err := invoke(args...); code != 3 {
			t.Errorf("%v: %d %s", args, code, err)
		}
	}
	data, _ := os.ReadFile(file)
	if string(data) != html {
		t.Fatal("input was overwritten")
	}
	code, out, _ := invoke("--html-file", file, "--base-url", "https://example.test", "--complete=false")
	if code != 0 || out != "/a.js\n" {
		t.Fatalf("%d %q", code, out)
	}
	if code, _, _ := invoke("--html-file", file, "--base-url", "https://example.test", "--max-body-size=5"); code != 1 {
		t.Fatalf("limit code=%d", code)
	}
}
