package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestIncrementalRevalidation(t *testing.T) {
	for _, mode := range []string{"etag", "last-modified", "identical-200", "no-store", "vary-star"} {
		t.Run(mode, func(t *testing.T) {
			var requests, conditional atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/" {
					fmt.Fprint(w, "<script src='/s.js'></script><script>inline()</script>")
					return
				}
				requests.Add(1)
				w.Header().Set("ETag", `"v1"`)
				if mode == "last-modified" {
					w.Header().Del("ETag")
					w.Header().Set("Last-Modified", "Mon, 07 Sep 2026 12:00:00 GMT")
				}
				if mode == "no-store" {
					w.Header().Set("Cache-Control", "private, no-store")
				}
				if mode == "vary-star" {
					w.Header().Add("Vary", "Accept-Encoding")
					w.Header().Add("Vary", "*")
				}
				if r.Header.Get("If-None-Match") != "" || r.Header.Get("If-Modified-Since") != "" {
					conditional.Add(1)
					if mode != "identical-200" {
						w.WriteHeader(304)
						return
					}
				}
				fmt.Fprint(w, "script()")
			}))
			defer srv.Close()
			dir := t.TempDir()
			args := []string{"-u", srv.URL, "--save", "--jsonl", "--incremental", "--manifest", filepath.Join(dir, "manifest.jsonl"), "--output-dir", filepath.Join(dir, "scripts")}
			code, out, diagnostic := invoke(args...)
			if code != 0 {
				t.Fatalf("%d %s", code, diagnostic)
			}
			first := decodeSources(t, out)
			for run := 0; run < 2; run++ {
				code, out, diagnostic = invoke(args...)
				if code != 0 {
					t.Fatalf("%d %s", code, diagnostic)
				}
				items := decodeSources(t, out)
				if len(items) != 2 {
					t.Fatalf("%#v", items)
				}
				for i, item := range items {
					if item.Path != first[i].Path || item.SHA256 != first[i].SHA256 || !item.Reused {
						t.Fatalf("not reused: %#v", item)
					}
				}
				wantStatus := 304
				if mode == "identical-200" || mode == "no-store" || mode == "vary-star" {
					wantStatus = 200
				}
				if items[0].Status != wantStatus {
					t.Fatalf("status=%d", items[0].Status)
				}
			}
			wantConditional := int32(2)
			if mode == "no-store" || mode == "vary-star" {
				wantConditional = 0
			}
			if requests.Load() != 3 || conditional.Load() != wantConditional {
				t.Fatalf("requests=%d conditional=%d", requests.Load(), conditional.Load())
			}
		})
	}
}

func TestIncrementalChangedOrMissingContent(t *testing.T) {
	for _, mode := range []string{"changed", "missing", "corrupt", "removed-during-request", "headers"} {
		t.Run(mode, func(t *testing.T) {
			var phase, requests atomic.Int32
			var oldPath atomic.Value
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/" {
					fmt.Fprint(w, "<script src='/s.js'></script>")
					return
				}
				requests.Add(1)
				w.Header().Set("ETag", `"v1"`)
				if phase.Load() == 1 && r.Header.Get("If-None-Match") != "" {
					if mode == "missing" || mode == "corrupt" || mode == "headers" {
						t.Error("sent validators for invalid cache")
					}
					if mode == "removed-during-request" {
						os.Remove(oldPath.Load().(string))
						w.WriteHeader(304)
						return
					}
				}
				if phase.Load() == 1 && mode == "changed" {
					w.Header().Set("ETag", `"v2"`)
					fmt.Fprint(w, "changed()")
					return
				}
				fmt.Fprint(w, "script()")
			}))
			defer srv.Close()
			dir := t.TempDir()
			args := []string{"-u", srv.URL, "--save", "--jsonl", "--incremental", "--manifest", filepath.Join(dir, "manifest.jsonl"), "--output-dir", filepath.Join(dir, "scripts")}
			code, out, diagnostic := invoke(args...)
			if code != 0 {
				t.Fatalf("%d %s", code, diagnostic)
			}
			first := decodeSources(t, out)[0]
			oldPath.Store(first.Path)
			if mode == "missing" {
				if err := os.Remove(first.Path); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "corrupt" {
				if err := os.WriteFile(first.Path, []byte("tampered"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "headers" {
				args = append(args, "-H", "Accept-Language: es")
			}
			phase.Store(1)
			code, out, diagnostic = invoke(args...)
			if code != 0 {
				t.Fatalf("%d %s", code, diagnostic)
			}
			second := decodeSources(t, out)[0]
			data, err := os.ReadFile(second.Path)
			if err != nil {
				t.Fatal(err)
			}
			want := "script()"
			if mode == "changed" {
				want = "changed()"
				if first.Path == second.Path || first.SHA256 == second.SHA256 {
					t.Fatal("changed version overwritten")
				}
			}
			if string(data) != want {
				t.Fatalf("%q", data)
			}
			wantRequests := int32(2)
			if mode == "removed-during-request" {
				wantRequests = 3
			}
			if requests.Load() != wantRequests {
				t.Fatalf("requests=%d", requests.Load())
			}
		})
	}
}

func TestIncrementalRedirectValidatorsStayWithResource(t *testing.T) {
	var changed atomic.Bool
	var validated atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			fmt.Fprint(w, "<script src='/redirect'></script>")
		case "/redirect":
			if r.Header.Get("If-None-Match") != "" {
				t.Error("validator leaked to redirect")
			}
			target := "/old.js"
			if changed.Load() {
				target = "/new.js"
			}
			http.Redirect(w, r, target, 302)
		case "/old.js":
			w.Header().Set("ETag", `"shared-tag"`)
			if r.Header.Get("If-None-Match") != "" {
				validated.Add(1)
				w.WriteHeader(304)
				return
			}
			fmt.Fprint(w, "old()")
		case "/new.js":
			if r.Header.Get("If-None-Match") != "" {
				t.Error("validator leaked to changed resource")
			}
			w.Header().Set("ETag", `"shared-tag"`)
			fmt.Fprint(w, "new()")
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	args := []string{"-u", srv.URL, "--save", "--jsonl", "--incremental", "--follow-redirect", "--manifest", filepath.Join(dir, "manifest.jsonl"), "--output-dir", filepath.Join(dir, "scripts")}
	var firstPath string
	for i := 0; i < 3; i++ {
		if i == 2 {
			changed.Store(true)
		}
		code, out, diagnostic := invoke(args...)
		if code != 0 {
			t.Fatalf("%d %s", code, diagnostic)
		}
		item := decodeSources(t, out)[0]
		if i == 0 {
			firstPath = item.Path
		}
		if i == 1 && (!item.Reused || item.Path != firstPath) {
			t.Fatal("unchanged redirect not reused")
		}
		if i == 2 && (item.Path == firstPath || item.FinalURL != srv.URL+"/new.js") {
			t.Fatal("changed target reused incorrectly")
		}
	}
	if validated.Load() != 1 {
		t.Fatal("missing final-target validator")
	}
}

func TestIncrementalOfflineAndManifestFailure(t *testing.T) {
	dir := t.TempDir()
	html, manifest := filepath.Join(dir, "page.html"), filepath.Join(dir, "manifest.jsonl")
	if err := os.WriteFile(html, []byte("<script>inline()</script><script src='/never.js'></script>"), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"--html-file", html, "--base-url", "http://127.0.0.1:1", "--save", "--jsonl", "--incremental", "--manifest", manifest, "--output-dir", filepath.Join(dir, "scripts")}
	for i := 0; i < 2; i++ {
		code, out, diagnostic := invoke(args...)
		if code != 0 {
			t.Fatalf("%d %s", code, diagnostic)
		}
		items := decodeSources(t, out)
		if items[0].Reused != (i == 1) || items[1].Path != "" || items[1].Status != 0 {
			t.Fatalf("%#v", items)
		}
	}
	before, _ := os.ReadFile(manifest)
	if err := os.Remove(html); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := invoke(args...); code != 1 {
		t.Fatalf("code=%d", code)
	}
	after, _ := os.ReadFile(manifest)
	if string(before) != string(after) {
		t.Fatal("failed run replaced previous manifest")
	}
	if err := os.WriteFile(manifest, []byte("invalid JSON"), 0600); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := invoke(args...); code != 1 {
		t.Fatalf("code=%d", code)
	}
	after, _ = os.ReadFile(manifest)
	if string(after) != "invalid JSON" {
		t.Fatal("invalid manifest was overwritten")
	}
	leftovers, _ := filepath.Glob(filepath.Join(dir, ".getjs-manifest-*"))
	if len(leftovers) != 0 {
		t.Fatalf("temporary manifests: %v", leftovers)
	}
}

func TestCachedFileConfinementAndLimits(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	path := filepath.Join(outside, "script.js")
	if err := os.WriteFile(path, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	file := savedFile{Path: path, Size: 4, SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("test")))}
	c := config{outputDir: dir, maxBody: 8}
	if verifiedFile(c, file) {
		t.Fatal("accepted file outside root")
	}
	inside := filepath.Join(dir, "inside.js")
	if err := os.WriteFile(inside, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	file.Path = inside
	if !verifiedFile(c, file) {
		t.Fatal("valid file rejected")
	}
	c.maxBody = 3
	if verifiedFile(c, file) {
		t.Fatal("oversized cache accepted")
	}
	c.maxBody = 8
	link := filepath.Join(dir, "link.js")
	if err := os.Symlink(path, link); err == nil {
		file.Path = link
		if verifiedFile(c, file) {
			t.Fatal("escaping symlink accepted")
		}
	}
}

func TestIncrementalArgumentValidation(t *testing.T) {
	for _, args := range [][]string{
		{"--incremental"},
		{"--incremental", "--save"},
		{"--incremental", "--save", "--manifest=m", "-H", "If-None-Match: value"},
		{"--incremental", "--save", "--manifest=m", "-H", "If-Modified-Since: value"},
	} {
		if code, _, diagnostic := invoke(args...); code != 3 {
			t.Fatalf("%v: %d %s", args, code, diagnostic)
		}
	}
}

func TestManifestDoesNotStoreHeaderValues(t *testing.T) {
	c := config{headers: http.Header{"Authorization": {"Bearer private-value"}}}
	u, _ := parseHTTPURL("https://example.test/page")
	entry := manifestEntry{downloadCache: downloadCache{Context: requestContext(c, u)}}
	data, err := json.Marshal(entry)
	if err != nil || strings.Contains(string(data), "private-value") || entry.Context == "" {
		t.Fatalf("%s %v", data, err)
	}
}
