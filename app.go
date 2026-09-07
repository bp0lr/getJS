package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type application struct {
	manifest     io.Writer
	manifestSeen map[string]bool
	c            config
	client       *http.Client
	out, errOut  io.Writer
	records      int
	writeErr     error
	cacheMu      sync.Mutex
	checks       map[string]*checkEntry
	emitted      map[string]bool
}

type checkEntry struct {
	done   chan struct{}
	err    error
	result resourceResult
}

type resourceResult struct {
	status   int
	finalURL string
	file     savedFile
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) (code int) {
	c, help, err := parseConfig(args, stderr)
	if help {
		return 0
	}
	if err != nil {
		fmt.Fprintln(stderr, "getJS:", err)
		return 3
	}
	if c.showVersion {
		if _, err := fmt.Fprintf(stdout, "getJS %s (%s)\n", version, commit); err != nil {
			return 1
		}
		return 0
	}
	if c.url == "" && c.input == "" && c.htmlFile == "" && stdin == nil {
		fmt.Fprintln(stderr, "getJS: no page URLs supplied")
		return 3
	}
	if c.input != "" && c.output != "" && sameFilePath(c.input, c.output) {
		fmt.Fprintln(stderr, "getJS: --input and --output must be different files")
		return 3
	}
	if c.htmlFile != "" && c.output != "" && sameFilePath(c.htmlFile, c.output) {
		fmt.Fprintln(stderr, "getJS: --html-file and --output must be different files")
		return 3
	}
	if c.manifest != "" {
		for _, other := range []string{c.input, c.htmlFile, c.output} {
			if other != "" && sameFilePath(c.manifest, other) {
				fmt.Fprintln(stderr, "getJS: --manifest must differ from input and output files")
				return 3
			}
		}
	}
	var manifest io.Writer
	if c.manifest != "" {
		f, err := os.Create(c.manifest)
		if err != nil {
			fmt.Fprintln(stderr, "getJS: manifest:", err)
			return 1
		}
		buf := bufio.NewWriter(f)
		manifest = buf
		defer func() {
			if err := errors.Join(buf.Flush(), f.Close()); err != nil {
				fmt.Fprintln(stderr, "getJS: manifest:", err)
				if code != 130 {
					code = 1
				}
			}
		}()
	}
	writer := stdout
	if c.output != "" {
		f, err := os.Create(c.output)
		if err != nil {
			fmt.Fprintln(stderr, "getJS: output:", err)
			return 1
		}
		buf := bufio.NewWriter(f)
		writer = io.MultiWriter(stdout, buf)
		defer func() {
			if err := errors.Join(buf.Flush(), f.Close()); err != nil {
				fmt.Fprintln(stderr, "getJS: output:", err)
				if code != 130 {
					code = 1
				}
			}
		}()
	}
	a := application{c: c, out: writer, errOut: stderr, checks: make(map[string]*checkEntry), emitted: make(map[string]bool), manifest: manifest, manifestSeen: make(map[string]bool)}
	if c.htmlFile == "" {
		a.client = newHTTPClient(c)
		defer a.client.CloseIdleConnections()
	}
	failed, succeeded, count := 0, 0, 0
	seenPages := make(map[string]bool)
	err = walkInputs(c, stdin, func(page string) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		key := page
		if u, err := parseHTTPURL(page); err == nil {
			key = canonicalURL(u)
		}
		if seenPages[key] {
			return nil
		}
		seenPages[key] = true
		count++
		var pageErr error
		if c.htmlFile != "" {
			pageErr = a.processHTMLFile(ctx, page)
		} else {
			pageErr = a.processPage(ctx, page)
		}
		if err := pageErr; err != nil {
			failed++
			fmt.Fprintln(stderr, "getJS:", err)
			var scriptErrors *scriptFailures
			if !errors.As(err, &scriptErrors) && a.writeErr == nil {
				if err := a.emit(source{Page: page, Kind: "page", Error: err.Error()}); err != nil {
					return err
				}
			}
		} else {
			succeeded++
		}
		if a.writeErr != nil {
			return a.writeErr
		}
		return nil
	})
	if a.writeErr != nil {
		return 1
	}
	if ctx.Err() != nil {
		return 130
	}
	if err != nil {
		failed++
		fmt.Fprintln(stderr, "getJS: input:", err)
	}
	if count == 0 && err == nil {
		fmt.Fprintln(stderr, "getJS: no page URLs supplied")
		return 3
	}
	if failed > 0 {
		if succeeded > 0 || a.records > 0 {
			return 2
		}
		return 1
	}
	return 0
}

func walkInputs(c config, stdin io.Reader, visit func(string) error) error {
	if c.htmlFile != "" {
		abs, err := filepath.Abs(c.htmlFile)
		if err != nil {
			return err
		}
		p := filepath.ToSlash(abs)
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		return visit((&url.URL{Scheme: "file", Path: p}).String())
	}
	read := func(r io.Reader) error {
		s := bufio.NewScanner(r)
		s.Buffer(make([]byte, 4096), 1024*1024)
		for s.Scan() {
			if line := strings.TrimSpace(s.Text()); line != "" {
				if err := visit(line); err != nil {
					return err
				}
			}
		}
		return s.Err()
	}
	if stdin != nil {
		if err := read(stdin); err != nil {
			return fmt.Errorf("stdin: %w", err)
		}
	}
	if c.input != "" {
		f, err := os.Open(c.input)
		if err != nil {
			return fmt.Errorf("input: %w", err)
		}
		err = read(f)
		closeErr := f.Close()
		if err := errors.Join(err, closeErr); err != nil {
			return fmt.Errorf("input: %w", err)
		}
	}
	if c.url != "" {
		return visit(strings.TrimSpace(c.url))
	}
	return nil
}

func sameFilePath(a, b string) bool {
	aa, _ := filepath.Abs(a)
	bb, _ := filepath.Abs(b)
	if aa == bb {
		return true
	}
	ai, ae := os.Stat(a)
	bi, be := os.Stat(b)
	return ae == nil && be == nil && os.SameFile(ai, bi)
}

func canonicalURL(u *url.URL) string {
	v := *u
	v.Scheme = strings.ToLower(v.Scheme)
	v.Host = strings.ToLower(v.Host)
	v.Fragment, v.RawFragment = "", ""
	return v.String()
}

func (a *application) processPage(ctx context.Context, raw string) error {
	u, err := parseHTTPURL(raw)
	if err != nil {
		return fmt.Errorf("page URL: %w", err)
	}
	if a.c.verbose {
		fmt.Fprintln(a.errOut, "getJS: reading", u.String())
	}
	resp, err := request(ctx, a.client, a.c, u, u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("page %s: HTTP %d", u, resp.StatusCode)
	}
	if resp.ContentLength > a.c.maxBody {
		return fmt.Errorf("page exceeds --max-body-size (%d bytes)", a.c.maxBody)
	}
	body := &io.LimitedReader{R: resp.Body, N: a.c.maxBody + 1}
	sources, err := extract(body, u.String(), resp.Request.URL)
	closeErr := resp.Body.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if body.N == 0 {
		return fmt.Errorf("page exceeds --max-body-size (%d bytes)", a.c.maxBody)
	}
	return a.processSources(ctx, sources, resp.Request.URL)
}

func (a *application) processSources(ctx context.Context, sources []source, origin *url.URL) error {
	if a.c.sameOriginOnly || len(a.c.include) > 0 || len(a.c.exclude) > 0 {
		filtered := make([]source, 0, len(sources))
		for _, s := range sources {
			if s.Kind == "inline" {
				filtered = append(filtered, s)
				continue
			}
			u, err := parseHTTPURL(s.URL)
			if err == nil && (!a.c.sameOriginOnly || sameOrigin(u, origin)) && a.c.matchesURL(s.URL) {
				filtered = append(filtered, s)
			}
		}
		sources = filtered
	}
	var failures []error
	// Batches bound both active work and the buffer needed for ordered output.
	for start := 0; start < len(sources); start += a.c.concurrency {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		end := min(start+a.c.concurrency, len(sources))
		errs := make([]error, end-start)
		results := append([]source(nil), sources[start:end]...)
		var wg sync.WaitGroup
		for i := start; i < end; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				s := &results[i-start]
				if s.Kind == "inline" {
					if a.c.save {
						var file savedFile
						file, errs[i-start] = saveScript(a.c, *s, strings.NewReader(s.Inline), i)
						s.Path, s.Size, s.SHA256 = file.Path, file.Size, file.SHA256
					}
				} else if a.c.htmlFile == "" && (a.c.resolve || a.c.save) {
					var result resourceResult
					result, errs[i-start] = a.cachedCheck(ctx, *s, origin, i)
					s.Status, s.FinalURL, s.Path, s.Size, s.SHA256 = result.status, result.finalURL, result.file.Path, result.file.Size, result.file.SHA256
				}
			}(i)
		}
		wg.Wait()
		for i := start; i < end; i++ {
			if errs[i-start] != nil {
				failures = append(failures, errs[i-start])
				results[i-start].Error = errs[i-start].Error()
				if err := a.emit(results[i-start]); err != nil {
					return err
				}
				continue
			}
			if err := a.emit(results[i-start]); err != nil {
				return err
			}
		}
	}
	if len(failures) > 0 {
		return &scriptFailures{err: errors.Join(failures...)}
	}
	return nil
}

type scriptFailures struct{ err error }

func (e *scriptFailures) Error() string { return e.err.Error() }
func (e *scriptFailures) Unwrap() error { return e.err }

func (a *application) cachedCheck(ctx context.Context, s source, origin *url.URL, index int) (resourceResult, error) {
	u, _ := parseHTTPURL(s.URL)
	key := canonicalURL(u)
	if len(a.c.headers) > 0 {
		key += "\x00" + origin.Scheme + "://" + origin.Host
	}
	// Saved files are grouped by source page; checks can be shared across pages.
	if a.c.save {
		key += "\x00" + s.Page
	}
	a.cacheMu.Lock()
	if entry, ok := a.checks[key]; ok {
		a.cacheMu.Unlock()
		select {
		case <-entry.done:
			return entry.result, entry.err
		case <-ctx.Done():
			return resourceResult{}, ctx.Err()
		}
	}
	entry := &checkEntry{done: make(chan struct{})}
	a.checks[key] = entry
	a.cacheMu.Unlock()
	entry.result, entry.err = a.checkScript(ctx, s, origin, index)
	close(entry.done)
	return entry.result, entry.err
}

func (a *application) checkScript(ctx context.Context, s source, origin *url.URL, index int) (result resourceResult, err error) {
	if a.c.sameOriginOnly {
		ctx = context.WithValue(ctx, originScopeKey{}, origin)
	}
	u, _ := parseHTTPURL(s.URL)
	resp, err := request(ctx, a.client, a.c, u, origin)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	result.status, result.finalURL = resp.StatusCode, resp.Request.URL.String()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotModified {
		return result, fmt.Errorf("script %s: HTTP %d", s.URL, resp.StatusCode)
	}
	if resp.ContentLength > a.c.maxBody {
		return result, fmt.Errorf("script exceeds --max-body-size (%d bytes)", a.c.maxBody)
	}
	if a.c.save {
		if resp.StatusCode == http.StatusNotModified {
			return result, fmt.Errorf("script %s: HTTP 304 has no body to save", s.URL)
		}
		result.file, err = saveScript(a.c, s, resp.Body, index)
		return result, err
	}
	// Bound draining so that a large script does not monopolize a worker.
	n, err := io.Copy(io.Discard, io.LimitReader(resp.Body, min(64*1024, a.c.maxBody+1)))
	if n > a.c.maxBody {
		return result, fmt.Errorf("script exceeds --max-body-size (%d bytes)", a.c.maxBody)
	}
	return result, err
}

func (a *application) emit(s source) error {
	if a.manifest != nil && s.Error == "" && s.Path != "" && !a.manifestSeen[s.Path] {
		entry := manifestEntry{Page: s.Page, URL: s.URL, Kind: s.Kind, Path: s.Path, Size: s.Size, SHA256: s.SHA256}
		if err := json.NewEncoder(a.manifest).Encode(entry); err != nil {
			a.writeErr = err
			return err
		}
		a.manifestSeen[s.Path] = true
	}
	if a.c.jsonl {
		if err := json.NewEncoder(a.out).Encode(s); err != nil {
			a.writeErr = err
			return err
		}
		if s.Error == "" {
			a.records++
		}
		return nil
	}
	if s.Error != "" {
		return nil
	}
	if s.Kind == "inline" {
		if s.Path != "" {
			a.records++
		}
		return nil
	}
	u, _ := parseHTTPURL(s.URL)
	key := canonicalURL(u)
	if a.emitted[key] {
		return nil
	}
	value := s.URL
	if !a.c.complete {
		value = s.Raw
	}
	_, err := fmt.Fprintln(a.out, value)
	if err != nil {
		a.writeErr = err
		return err
	}
	a.records++
	a.emitted[key] = true
	return nil
}
