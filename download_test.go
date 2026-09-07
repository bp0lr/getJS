package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadsPreserveContentAndSeparateURLNames(t *testing.T) {
	c := config{outputDir: t.TempDir(), maxBody: 1024}
	s := source{Page: "https://example.test/page", URL: "https://example.test/CON.js?v=1", Kind: "script"}
	first, err := saveScript(c, s, strings.NewReader("first"), 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := saveScript(c, s, strings.NewReader("second"), 0)
	if err != nil {
		t.Fatal(err)
	}
	s.URL = "https://example.test/CON.js?v=2"
	third, err := saveScript(c, s, strings.NewReader("third"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if first.Path == second.Path || first.Path == third.Path || second.Path == third.Path {
		t.Fatal("colliding paths")
	}
	for _, item := range []struct {
		file savedFile
		want string
	}{{first, "first"}, {second, "second"}, {third, "third"}} {
		data, err := os.ReadFile(item.file.Path)
		if err != nil || string(data) != item.want || item.file.Size != int64(len(data)) {
			t.Fatalf("%#v %s %v", item.file, data, err)
		}
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("reader failed") }

func TestDownloadFailureLeavesNoFiles(t *testing.T) {
	for _, reader := range []io.Reader{strings.NewReader("too large"), failingReader{}} {
		c := config{outputDir: t.TempDir(), maxBody: 3}
		_, err := saveScript(c, source{Page: "https://example.test", Kind: "inline"}, reader, 0)
		if err == nil {
			t.Fatal("expected failure")
		}
		filepath.WalkDir(c.outputDir, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				t.Fatal(err)
			}
			if !entry.IsDir() {
				t.Errorf("left file %s", path)
			}
			return nil
		})
	}
}

func decodeSources(t *testing.T, output string) []source {
	t.Helper()
	var result []source
	dec := json.NewDecoder(strings.NewReader(output))
	for {
		var s source
		err := dec.Decode(&s)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("invalid JSONL: %s: %v", output, err)
		}
		result = append(result, s)
	}
	return result
}

func TestJSONLDownloadsAndPartialErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			fmt.Fprint(w, "<script src='/ok.js'></script><script>inline()</script><script src='/missing.js'></script>")
		case "/ok.js":
			fmt.Fprint(w, "script()")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	code, out, err := invoke("-u", srv.URL, "--jsonl", "--save", "--resolve=false", "--output-dir", t.TempDir())
	if code != 2 {
		t.Fatalf("code=%d %s", code, err)
	}
	items := decodeSources(t, out)
	if len(items) != 3 {
		t.Fatalf("items=%#v", items)
	}
	if items[0].Status != 200 || items[0].Path == "" || items[0].FinalURL != srv.URL+"/ok.js" {
		t.Fatalf("%#v", items[0])
	}
	if items[1].Kind != "inline" || items[1].Path == "" || strings.Contains(out, "inline()") {
		t.Fatalf("%#v", items[1])
	}
	if items[2].Status != 404 || items[2].Error == "" || items[2].Path != "" {
		t.Fatalf("%#v", items[2])
	}
}

func TestBodyLimitsAnd304Downloads(t *testing.T) {
	t.Run("HTML", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, strings.Repeat("x", 100)) }))
		defer srv.Close()
		code, out, _ := invoke("-u", srv.URL, "--max-body-size=20", "--jsonl")
		items := decodeSources(t, out)
		if code != 1 || len(items) != 1 || items[0].Kind != "page" || items[0].Error == "" {
			t.Fatalf("%d %#v", code, items)
		}
	})
	t.Run("chunked script", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				fmt.Fprint(w, "<script src='/s.js'></script>")
				return
			}
			w.(http.Flusher).Flush()
			fmt.Fprint(w, strings.Repeat("x", 100))
		}))
		defer srv.Close()
		code, out, _ := invoke("-u", srv.URL, "--max-body-size=50", "--save", "--jsonl", "--output-dir", t.TempDir())
		items := decodeSources(t, out)
		if code != 1 || len(items) != 1 || items[0].Path != "" || items[0].Error == "" {
			t.Fatalf("%d %#v", code, items)
		}
	})
	t.Run("304", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				fmt.Fprint(w, "<script src='/s.js'></script>")
				return
			}
			w.WriteHeader(304)
		}))
		defer srv.Close()
		code, out, _ := invoke("-u", srv.URL, "--save", "--jsonl", "--output-dir", t.TempDir())
		items := decodeSources(t, out)
		if code != 1 || len(items) != 1 || items[0].Status != 304 || items[0].Path != "" {
			t.Fatalf("%d %#v", code, items)
		}
	})
}

func TestVersionWithoutInput(t *testing.T) {
	if code, out, _ := invoke("--version"); code != 0 || !strings.Contains(out, "getJS dev") {
		t.Fatalf("%d %q", code, out)
	}
}
