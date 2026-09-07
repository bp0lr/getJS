package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) &&
		strings.EqualFold(a.Hostname(), b.Hostname()) && effectivePort(a) == effectivePort(b)
}

func effectivePort(u *url.URL) string {
	if u.Port() != "" {
		return u.Port()
	}
	if strings.EqualFold(u.Scheme, "https") {
		return "443"
	}
	return "80"
}

type originScopeKey struct{}

func newHTTPClient(c config) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: c.insecure}
	tr.TLSHandshakeTimeout = c.timeout
	tr.ResponseHeaderTimeout = c.timeout
	tr.IdleConnTimeout = 30 * time.Second
	tr.MaxIdleConns = max(32, c.concurrency*2)
	tr.MaxIdleConnsPerHost = max(1, c.perHost)
	tr.MaxConnsPerHost = max(1, c.perHost)
	if c.proxy != "" {
		p, _ := url.Parse(c.proxy) // Validated by parseConfig.
		tr.Proxy = http.ProxyURL(p)
	}
	return &http.Client{
		Transport: &limitedTransport{base: tr, limit: max(1, c.perHost), hosts: make(map[string]chan struct{})},
		Timeout:   c.timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if scope, ok := req.Context().Value(originScopeKey{}).(*url.URL); ok && !sameOrigin(req.URL, scope) {
				return fmt.Errorf("script redirect leaves the page origin")
			}
			if !c.follow {
				return http.ErrUseLastResponse
			}
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			if _, err := parseHTTPURL(req.URL.String()); err != nil {
				return err
			}
			// Once a chain changes origin, custom headers remain removed.
			for _, prev := range via {
				if !sameOrigin(prev.URL, req.URL) {
					for name := range c.headers {
						req.Header.Del(name)
					}
					req.Host = ""
					break
				}
			}
			applyConditional(req)
			return nil
		},
	}
}

// Keep the per-host limit through body consumption, also for HTTP/2 streams.
type limitedTransport struct {
	base  *http.Transport
	limit int
	mu    sync.Mutex
	hosts map[string]chan struct{}
}

func (t *limitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	key := strings.ToLower(req.URL.Hostname())
	t.mu.Lock()
	sem := t.hosts[key]
	if sem == nil {
		sem = make(chan struct{}, t.limit)
		t.hosts[key] = sem
	}
	t.mu.Unlock()
	select {
	case sem <- struct{}{}:
	case <-req.Context().Done():
		return nil, req.Context().Err()
	}
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		<-sem
		return nil, err
	}
	resp.Body = &releaseBody{ReadCloser: resp.Body, release: func() { <-sem }}
	return resp, nil
}

func (t *limitedTransport) CloseIdleConnections() { t.base.CloseIdleConnections() }

type releaseBody struct {
	io.ReadCloser
	once    sync.Once
	release func()
	err     error
}

func (b *releaseBody) Close() error {
	b.once.Do(func() { b.err = b.ReadCloser.Close(); b.release() })
	return b.err
}

func request(ctx context.Context, client *http.Client, c config, target, origin *url.URL) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	if sameOrigin(target, origin) {
		for name, values := range c.headers {
			if strings.EqualFold(name, "Host") {
				req.Host = values[len(values)-1]
			} else {
				req.Header[name] = append([]string(nil), values...)
			}
		}
	}
	applyConditional(req)
	return client.Do(req)
}
