package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestOriginComparison(t *testing.T) {
	a, _ := url.Parse("https://EXAMPLE.test/path")
	for _, tc := range []struct {
		raw  string
		same bool
	}{
		{"https://example.test:443/other", true},
		{"http://example.test:443", false},
		{"https://example.test:444", false},
		{"https://sub.example.test", false},
		{"https://example.test.evil.test", false},
	} {
		b, _ := url.Parse(tc.raw)
		if sameOrigin(a, b) != tc.same {
			t.Errorf("%s", tc.raw)
		}
	}
}

func TestSameOriginSkipsExternalAndBlocksRedirect(t *testing.T) {
	var hits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); fmt.Fprint(w, "external") }))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			fmt.Fprintf(w, "<script src='%s/direct.js'></script><script src='/local.js'></script><script src='/redirect.js'></script>", other.URL)
		case "/redirect.js":
			http.Redirect(w, r, other.URL+"/redirected.js", 302)
		default:
			fmt.Fprint(w, "local")
		}
	}))
	defer srv.Close()
	code, out, diagnostics := invoke("-u", srv.URL, "--same-origin", "-f")
	if code != 2 || hits.Load() != 0 || out != srv.URL+"/local.js\n" || !strings.Contains(diagnostics, "origin") {
		t.Fatalf("%d %q %q hits=%d", code, out, diagnostics, hits.Load())
	}
}

func TestSameOriginUsesFinalPageOriginAndHTMLBaseCannotExpandIt(t *testing.T) {
	var html string
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, html) }))
	defer final.Close()
	start := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, final.URL+"/page", 302) }))
	defer start.Close()
	html = fmt.Sprintf("<base href='%s/'><script src='off.js'></script><script src='%s/ok.js'></script>", start.URL, final.URL)
	code, out, err := invoke("-u", start.URL, "-f", "--same-origin", "--resolve=false")
	if code != 0 || out != final.URL+"/ok.js\n" {
		t.Fatalf("%d %q %s", code, out, err)
	}
}
