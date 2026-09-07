package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"strings"
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

func newHTTPClient(c config) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: c.insecure}
	tr.TLSHandshakeTimeout = c.timeout
	tr.ResponseHeaderTimeout = c.timeout
	tr.IdleConnTimeout = 30 * time.Second
	if c.proxy != "" {
		p, _ := url.Parse(c.proxy) // Validated by parseConfig.
		tr.Proxy = http.ProxyURL(p)
	}
	return &http.Client{
		Transport: tr,
		Timeout:   c.timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
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
			return nil
		},
	}
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
	return client.Do(req)
}
