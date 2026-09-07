package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/pflag"
)

type config struct {
	concurrency, perHost                                         int
	url, input, output, proxy                                    string
	headers                                                      http.Header
	complete, resolve, save, follow, verbose, noColors, insecure bool
	timeout                                                      time.Duration
}

func parseConfig(args []string, stderr io.Writer) (config, bool, error) {
	var c config
	var headers []string
	f := pflag.NewFlagSet("getJS", pflag.ContinueOnError)
	f.SetOutput(stderr)
	f.StringVarP(&c.url, "url", "u", "", "Page URL")
	f.StringVarP(&c.input, "input", "i", "", "File containing one page URL per line")
	f.StringVarP(&c.output, "output", "o", "", "Write results to a file as well as stdout")
	f.StringVarP(&c.proxy, "proxy", "p", "", "HTTP or HTTPS proxy URL")
	f.StringArrayVarP(&headers, "header", "H", nil, "Page and same-origin script header (repeatable)")
	f.BoolVarP(&c.complete, "complete", "c", true, "Print absolute script URLs")
	f.BoolVarP(&c.resolve, "resolve", "r", true, "Check script URLs")
	f.BoolVarP(&c.save, "save", "s", false, "Save scripts and inline code")
	f.BoolVarP(&c.follow, "follow-redirect", "f", false, "Follow up to 10 redirects")
	f.BoolVarP(&c.verbose, "verbose", "v", false, "Write progress to stderr")
	f.BoolVarP(&c.noColors, "nocolors", "n", false, "Compatibility option; output is plain text")
	f.BoolVar(&c.insecure, "insecure", false, "Disable TLS certificate verification")
	f.DurationVar(&c.timeout, "timeout", 15*time.Second, "Maximum duration of each HTTP request")
	f.IntVar(&c.concurrency, "concurrency", 4, "Maximum simultaneous script tasks")
	f.IntVar(&c.perHost, "per-host", 2, "Maximum simultaneous HTTP requests per hostname")
	if err := f.Parse(args); err != nil {
		return c, err == pflag.ErrHelp, err
	}
	if f.NArg() != 0 {
		return c, false, fmt.Errorf("unexpected positional arguments; use --url or --input")
	}
	if c.timeout <= 0 {
		return c, false, fmt.Errorf("--timeout must be positive")
	}
	if c.concurrency < 1 || c.concurrency > 256 || c.perHost < 1 || c.perHost > 256 {
		return c, false, fmt.Errorf("--concurrency and --per-host must be between 1 and 256")
	}
	if c.resolve && !c.complete {
		return c, false, fmt.Errorf("--resolve requires --complete=true")
	}
	if c.url != "" {
		if _, err := parseHTTPURL(c.url); err != nil {
			return c, false, err
		}
	}
	if c.proxy != "" {
		p, err := url.Parse(c.proxy)
		if err != nil || p == nil || p.Hostname() == "" || (p.Scheme != "http" && p.Scheme != "https") {
			return c, false, fmt.Errorf("--proxy requires an absolute HTTP or HTTPS URL")
		}
		if err := validPort(p); err != nil {
			return c, false, fmt.Errorf("invalid proxy port")
		}
	}
	c.headers = make(http.Header)
	for _, h := range headers {
		name, value, found := strings.Cut(h, ":")
		name, value = strings.TrimSpace(name), strings.TrimSpace(value)
		if !found || !validHeaderName(name) || strings.ContainsAny(value, "\r\n\x00") {
			return c, false, fmt.Errorf("invalid --header; expected Name: value")
		}
		c.headers.Add(name, value)
	}
	return c, false, nil
}

func validHeaderName(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			continue
		}
		if !strings.ContainsRune("!#$%&'*+-.^_"+string(rune(96))+"|~", c) {
			return false
		}
	}
	return true
}

func validPort(u *url.URL) error {
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("invalid port")
		}
	}
	return nil
}

func parseHTTPURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return nil, fmt.Errorf("expected an absolute HTTP or HTTPS URL without embedded credentials")
	}
	if err := validPort(u); err != nil {
		return nil, err
	}
	u.Fragment, u.RawFragment = "", ""
	return u, nil
}
