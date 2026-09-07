package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/pflag"
)

type config struct {
	incremental                                                  bool
	manifest                                                     string
	htmlFile, baseURL                                            string
	include, exclude                                             []*regexp.Regexp
	sameOriginOnly                                               bool
	jsonl, showVersion                                           bool
	outputDir                                                    string
	maxBody                                                      int64
	concurrency, perHost                                         int
	url, input, output, proxy                                    string
	headers                                                      http.Header
	complete, resolve, save, follow, verbose, noColors, insecure bool
	timeout                                                      time.Duration
}

func parseConfig(args []string, stderr io.Writer) (config, bool, error) {
	var c config
	var headers []string
	var includes, excludes []string
	f := pflag.NewFlagSet("getJS", pflag.ContinueOnError)
	f.SetOutput(stderr)
	f.StringVarP(&c.url, "url", "u", "", "Page URL")
	f.StringVarP(&c.input, "input", "i", "", "File containing one page URL per line")
	f.StringVar(&c.htmlFile, "html-file", "", "Parse a local HTML file without network requests")
	f.StringVar(&c.baseURL, "base-url", "", "Absolute HTTP(S) base URL for --html-file")
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
	f.BoolVar(&c.jsonl, "jsonl", false, "Write one JSON object per discovery or error")
	f.BoolVar(&c.sameOriginOnly, "same-origin", false, "Only process scripts from the final page origin")
	f.StringArrayVar(&includes, "include", nil, "Include script URLs matching a Go regexp (repeatable, any match)")
	f.StringArrayVar(&excludes, "exclude", nil, "Exclude script URLs matching a Go regexp (repeatable, takes precedence)")
	f.BoolVar(&c.showVersion, "version", false, "Print build version")
	f.StringVar(&c.outputDir, "output-dir", "download", "Directory for script downloads")
	f.StringVar(&c.manifest, "manifest", "", "Write a JSONL manifest of completed downloads (requires --save)")
	f.BoolVar(&c.incremental, "incremental", false, "Reuse verified files from --manifest (requires --save and --manifest)")
	f.Int64Var(&c.maxBody, "max-body-size", 10*1024*1024, "Maximum HTML or downloaded script size in bytes")
	if err := f.Parse(args); err != nil {
		return c, err == pflag.ErrHelp, err
	}
	if f.NArg() != 0 {
		return c, false, fmt.Errorf("unexpected positional arguments; use --url or --input")
	}
	if c.showVersion {
		return c, false, nil
	}
	if c.manifest != "" && !c.save {
		return c, false, fmt.Errorf("--manifest requires --save")
	}
	if c.incremental && (!c.save || c.manifest == "") {
		return c, false, fmt.Errorf("--incremental requires --save and --manifest")
	}
	if c.htmlFile != "" {
		if c.url != "" || c.input != "" {
			return c, false, fmt.Errorf("--html-file cannot be combined with --url or --input")
		}
		if f.Changed("resolve") && c.resolve {
			return c, false, fmt.Errorf("--html-file is offline; --resolve=true is not allowed")
		}
		c.resolve = false
		if _, err := parseHTTPURL(c.baseURL); err != nil {
			return c, false, fmt.Errorf("--html-file requires a valid --base-url: %w", err)
		}
	} else if c.baseURL != "" {
		return c, false, fmt.Errorf("--base-url requires --html-file")
	}
	for _, patterns := range []struct {
		name   string
		values []string
		dest   *[]*regexp.Regexp
	}{{"include", includes, &c.include}, {"exclude", excludes, &c.exclude}} {
		for _, pattern := range patterns.values {
			re, err := regexp.Compile(pattern)
			if err != nil {
				return c, false, fmt.Errorf("--%s: invalid regexp %q: %w", patterns.name, pattern, err)
			}
			*patterns.dest = append(*patterns.dest, re)
		}
	}
	if c.maxBody < 1 || c.maxBody > 1<<40 {
		return c, false, fmt.Errorf("--max-body-size must be between 1 and 1099511627776 bytes")
	}
	if strings.TrimSpace(c.outputDir) == "" {
		return c, false, fmt.Errorf("--output-dir cannot be empty")
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
	if c.incremental && (len(c.headers.Values("If-None-Match")) > 0 || len(c.headers.Values("If-Modified-Since")) > 0) {
		return c, false, fmt.Errorf("--incremental manages conditional headers; remove explicit If-None-Match and If-Modified-Since headers")
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
