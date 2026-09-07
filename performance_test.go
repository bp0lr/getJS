package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConcurrencyDeduplicationAndConnections(t *testing.T) {
	var active, peak, requests, connections atomic.Int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/" || r.URL.Path == "/second" {
			for i := 0; i < 8; i++ {
				fmt.Fprintf(w, "<script src='/s%d.js'></script><script src='/s%d.js'></script>", i, i)
			}
			return
		}
		n := active.Add(1)
		for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
		}
		defer active.Add(-1)
		time.Sleep(5 * time.Millisecond)
		fmt.Fprint(w, "small reusable body")
	}))
	srv.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateNew {
			connections.Add(1)
		}
	}
	srv.Start()
	defer srv.Close()
	var out, diagnostics bytes.Buffer
	input := strings.NewReader(srv.URL + "\n" + srv.URL + "\n" + srv.URL + "/second\n")
	code := run(context.Background(), []string{"--concurrency=8", "--per-host=2"}, input, &out, &diagnostics)
	if code != 0 {
		t.Fatalf("%d %s", code, diagnostics.String())
	}
	if requests.Load() != 10 {
		t.Fatalf("requests=%d, want two pages + eight scripts", requests.Load())
	}
	if peak.Load() < 2 || peak.Load() > 2 {
		t.Fatalf("peak=%d", peak.Load())
	}
	if connections.Load() > 2 {
		t.Fatalf("connections=%d; expected reuse", connections.Load())
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 8 {
		t.Fatalf("lines=%v", lines)
	}
	for i, line := range lines {
		if line != fmt.Sprintf("%s/s%d.js", srv.URL, i) {
			t.Fatalf("order=%v", lines)
		}
	}
}

func TestPerHostOneDoesNotHoldPageConnection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			fmt.Fprint(w, "<script src='/s.js'></script>")
			return
		}
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()
	if code, _, err := invoke("-u", srv.URL, "--per-host=1", "--timeout=1s"); code != 0 {
		t.Fatalf("code=%d %s", code, err)
	}
}

func TestPerHostLimitWithHTTP2(t *testing.T) {
	var active, peak atomic.Int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 2 {
			t.Errorf("expected HTTP/2, got %s", r.Proto)
		}
		if r.URL.Path == "/" {
			for i := 0; i < 6; i++ {
				fmt.Fprintf(w, "<script src='/s%d.js'></script>", i)
			}
			return
		}
		n := active.Add(1)
		for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
		}
		defer active.Add(-1)
		time.Sleep(5 * time.Millisecond)
		fmt.Fprint(w, "ok")
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()
	if code, _, diagnostics := invoke("-u", srv.URL, "--insecure", "--concurrency=6", "--per-host=2"); code != 0 {
		t.Fatalf("%d %s", code, diagnostics)
	}
	if peak.Load() != 2 {
		t.Fatalf("peak=%d", peak.Load())
	}
}

func TestContinuousWorkersAdvancePastSlowFirstResource(t *testing.T) {
	thirdStarted := make(chan struct{})
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			fmt.Fprint(w, "<script src='/slow.js'></script><script src='/fast.js'></script><script src='/third.js'></script>")
			return
		case "/slow.js":
			select {
			case <-thirdStarted:
			case <-r.Context().Done():
				return
			}
		case "/third.js":
			once.Do(func() { close(thirdStarted) })
		}
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()
	code, out, err := invoke("-u", srv.URL, "--concurrency=2", "--per-host=2", "--timeout=2s")
	want := srv.URL + "/slow.js\n" + srv.URL + "/fast.js\n" + srv.URL + "/third.js\n"
	if code != 0 || out != want {
		t.Fatalf("code=%d output=%q error=%s", code, out, err)
	}
}

func TestDuplicateReferencesDoNotOccupyWorkers(t *testing.T) {
	nextStarted := make(chan struct{})
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			fmt.Fprint(w, "<script src='/slow.js'></script><script src='/slow.js'></script><script src='/next.js'></script>")
			return
		case "/slow.js":
			select {
			case <-nextStarted:
			case <-r.Context().Done():
				return
			}
		case "/next.js":
			once.Do(func() { close(nextStarted) })
		}
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()
	code, out, err := invoke("-u", srv.URL, "--concurrency=2", "--per-host=2", "--timeout=2s", "--jsonl")
	got := decodeSources(t, out)
	if code != 0 || len(got) != 3 || got[0].URL != got[1].URL || got[0].Metadata.Index == got[1].Metadata.Index {
		t.Fatalf("%d %#v %s", code, got, err)
	}
}

func BenchmarkPipeline(b *testing.B) {
	var requests atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/" {
			for i := 0; i < 16; i++ {
				fmt.Fprintf(w, "<script src='/s%d.js'></script><script src='/s%d.js'></script>", i, i)
			}
			return
		}
		time.Sleep(2 * time.Millisecond)
		fmt.Fprint(w, strings.Repeat("x", 1024))
	}))
	defer srv.Close()
	for _, workers := range []int{1, 4, 8} {
		b.Run(strconv.Itoa(workers), func(b *testing.B) {
			start := requests.Load()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				code := run(context.Background(), []string{"-u", srv.URL, "--concurrency=" + strconv.Itoa(workers), "--per-host=" + strconv.Itoa(workers)}, nil, io.Discard, io.Discard)
				if code != 0 {
					b.Fatalf("code=%d", code)
				}
			}
			b.ReportMetric(float64(requests.Load()-start)/float64(b.N), "requests/op")
		})
	}
}

// Compare methods without assuming that HEAD support matches GET support.
func BenchmarkCheckMethod(b *testing.B) {
	for _, rejectHEAD := range []bool{false, true} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(time.Millisecond)
			if rejectHEAD && r.Method == http.MethodHead {
				w.WriteHeader(405)
				return
			}
			w.Header().Set("Content-Length", "1024")
			if r.Method != http.MethodHead {
				fmt.Fprint(w, strings.Repeat("x", 1024))
			}
		}))
		for _, useHEAD := range []bool{false, true} {
			b.Run(fmt.Sprintf("rejectHEAD=%v/HEAD=%v", rejectHEAD, useHEAD), func(b *testing.B) {
				client := srv.Client()
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					method := http.MethodGet
					if useHEAD {
						method = http.MethodHead
					}
					req, _ := http.NewRequest(method, srv.URL, nil)
					resp, err := client.Do(req)
					if err != nil {
						b.Fatal(err)
					}
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					if useHEAD && resp.StatusCode == 405 {
						resp, err = client.Get(srv.URL)
						if err != nil {
							b.Fatal(err)
						}
						io.Copy(io.Discard, resp.Body)
						resp.Body.Close()
					}
				}
			})
		}
		srv.Close()
	}
}
