package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestURLFiltersApplyBeforeRequests(t *testing.T) {
	var scripts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			fmt.Fprint(w, "<script src='/app.js?v=1'></script><script src='/app-vendor.js'></script><script src='/other.js'></script><script src='/worker.js'></script>")
			return
		}
		if r.URL.Path != "/app.js" && r.URL.Path != "/worker.js" {
			t.Errorf("unexpected request %s", r.URL)
		}
		scripts.Add(1)
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()
	code, out, err := invoke("-u", srv.URL, "--include", "app", "--include", "worker", "--exclude", "vendor")
	if code != 0 || scripts.Load() != 2 || out != srv.URL+"/app.js?v=1\n"+srv.URL+"/worker.js\n" {
		t.Fatalf("%d %q %s hits=%d", code, out, err, scripts.Load())
	}
}

func TestFiltersUseAbsoluteURLsAndKeepInline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "<script src='/a.js'></script><script>inline()</script>")
	}))
	defer srv.Close()
	code, out, err := invoke("-u", srv.URL, "--resolve=false", "--complete=false", "--jsonl", "--include", "^https?://")
	if code != 0 {
		t.Fatalf("%d %s", code, err)
	}
	items := decodeSources(t, out)
	if len(items) != 2 || items[1].Kind != "inline" {
		t.Fatalf("%#v", items)
	}
}

func TestInvalidFilterFailsBeforeNetwork(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer srv.Close()
	code, out, err := invoke("-u", srv.URL, "--include", "[")
	if code != 3 || out != "" || calls.Load() != 0 || !strings.Contains(err, "regexp") {
		t.Fatalf("%d %q %q", code, out, err)
	}
}
