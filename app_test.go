package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func invoke(args ...string) (int, string, string) {
	var out, diagnostics bytes.Buffer
	code := run(context.Background(), args, nil, &out, &diagnostics)
	return code, out.String(), diagnostics.String()
}

func TestCLICompletesAgainstRedirectAndSeparatesOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			http.Redirect(w, r, "/nested/page.html", http.StatusFound)
		case "/nested/page.html":
			fmt.Fprint(w, "<script src='app.js'></script>")
		case "/nested/app.js":
			fmt.Fprint(w, "console.log('ok')")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	code, out, diagnostics := invoke("-u", srv.URL+"/start", "-f", "-v")
	if code != 0 || out != srv.URL+"/nested/app.js\n" || !strings.Contains(diagnostics, "reading") {
		t.Fatalf("code=%d out=%q stderr=%q", code, out, diagnostics)
	}
}

func TestCLIHeadersRespectOriginsAndPreserveColons(t *testing.T) {
	var leaked atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-Api-Key") != "" {
			leaked.Store(true)
		}
		fmt.Fprint(w, "ok")
	}))
	defer other.Close()
	var authorized atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer a:b" {
			authorized.Add(1)
		}
		switch r.URL.Path {
		case "/":
			fmt.Fprintf(w, "<script src='/local.js'></script><script src='%s/external.js'></script><script src='/redirect.js'></script>", other.URL)
		case "/redirect.js":
			http.Redirect(w, r, other.URL+"/redirected.js", 302)
		default:
			fmt.Fprint(w, "ok")
		}
	}))
	defer srv.Close()
	code, _, diagnostics := invoke("-u", srv.URL, "-f", "-v", "-H", "Authorization: Bearer a:b", "-H", "X-Api-Key: secret")
	if code != 0 || leaked.Load() || authorized.Load() != 3 {
		t.Fatalf("code=%d leak=%v requests=%d stderr=%s", code, leaked.Load(), authorized.Load(), diagnostics)
	}
	if strings.Contains(diagnostics, "secret") || strings.Contains(diagnostics, "a:b") {
		t.Fatal("credentials in logs")
	}
}

func TestTLSAndTimeout(t *testing.T) {
	tlsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "<html></html>") }))
	defer tlsServer.Close()
	if code, _, _ := invoke("-u", tlsServer.URL); code != 1 {
		t.Fatalf("untrusted TLS code=%d", code)
	}
	if code, _, err := invoke("-u", tlsServer.URL, "--insecure"); code != 0 {
		t.Fatalf("explicit insecure code=%d %s", code, err)
	}
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer slow.Close()
	start := time.Now()
	code, _, _ := invoke("-u", slow.URL, "--timeout=40ms")
	if code != 1 || time.Since(start) > 3*time.Second {
		t.Fatalf("timeout code=%d duration=%s", code, time.Since(start))
	}
}

func TestCLIExitCodesAndRawReferences(t *testing.T) {
	for _, args := range [][]string{nil, {"--url=bad"}, {"--timeout=0s"}, {"--complete=false"}, {"-H", "invalid"}, {"positional"}} {
		if code, out, _ := invoke(args...); code != 3 || out != "" {
			t.Errorf("args=%v code=%d stdout=%q", args, code, out)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			fmt.Fprint(w, "<script src='/ok.js'></script><script src='/missing.js'></script>")
			return
		}
		if r.URL.Path == "/missing.js" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()
	if code, out, _ := invoke("-u", srv.URL, "--resolve=false", "--complete=false"); code != 0 || out != "/ok.js\n/missing.js\n" {
		t.Fatalf("raw code=%d out=%q", code, out)
	}
	if code, out, err := invoke("-u", srv.URL); code != 2 || out != srv.URL+"/ok.js\n" || !strings.Contains(err, "404") {
		t.Fatalf("partial code=%d out=%q err=%q", code, out, err)
	}
}

func TestInputsAndOutput(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "pages.txt")
	if err := os.WriteFile(input, []byte(" \nhttps://example.test/a\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var pages []string
	err := walkInputs(config{input: input, url: "https://example.test/c"}, strings.NewReader("\n https://example.test/b \r\n"), func(s string) error { pages = append(pages, s); return nil })
	if err != nil || strings.Join(pages, ",") != "https://example.test/b,https://example.test/a,https://example.test/c" {
		t.Fatalf("%v %v", pages, err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "<script src='/a.js'></script>") }))
	defer srv.Close()
	dest := filepath.Join(dir, "out.txt")
	code, out, diagnostics := invoke("-u", srv.URL, "--resolve=false", "-o", dest)
	data, err := os.ReadFile(dest)
	if code != 0 || err != nil || string(data) != out {
		t.Fatalf("code=%d data=%s err=%v stderr=%s", code, data, err, diagnostics)
	}
	var stderr bytes.Buffer
	if code := run(context.Background(), []string{"-u", srv.URL, "--resolve=false"}, nil, brokenWriter{}, &stderr); code != 1 {
		t.Fatalf("writer failure code=%d", code)
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if code := run(ctx, []string{"-u", "https://example.test"}, nil, io.Discard, io.Discard); code != 130 {
		t.Fatalf("code=%d", code)
	}
}
