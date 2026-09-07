package main

import (
	"bufio"
	"context"
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
	c           config
	client      *http.Client
	out, errOut io.Writer
	records     int
	writeErr    error
	cacheMu     sync.Mutex
	checks      map[string]*checkEntry
	emitted     map[string]bool
}

type checkEntry struct {
	done chan struct{}
	err  error
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
	if c.url == "" && c.input == "" && stdin == nil {
		fmt.Fprintln(stderr, "getJS: no page URLs supplied")
		return 3
	}
	if c.input != "" && c.output != "" && sameFilePath(c.input, c.output) {
		fmt.Fprintln(stderr, "getJS: --input and --output must be different files")
		return 3
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
				if code == 0 {
					code = 1
				}
			}
		}()
	}
	a := application{c: c, client: newHTTPClient(c), out: writer, errOut: stderr, checks: make(map[string]*checkEntry), emitted: make(map[string]bool)}
	defer a.client.CloseIdleConnections()
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
		if err := a.processPage(ctx, page); err != nil {
			failed++
			fmt.Fprintln(stderr, "getJS:", err)
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
	sources, err := extract(resp.Body, u.String(), resp.Request.URL)
	closeErr := resp.Body.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return a.processSources(ctx, sources, resp.Request.URL)
}

func (a *application) processSources(ctx context.Context, sources []source, origin *url.URL) error {
	var failures []error
	// Batches bound both active work and the buffer needed for ordered output.
	for start := 0; start < len(sources); start += a.c.concurrency {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		end := min(start+a.c.concurrency, len(sources))
		errs := make([]error, end-start)
		var wg sync.WaitGroup
		for i := start; i < end; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				s := sources[i]
				if s.Kind == "inline" {
					if a.c.save {
						_, errs[i-start] = saveScript(s, strings.NewReader(s.Inline), i)
					}
				} else if a.c.resolve || a.c.save {
					errs[i-start] = a.cachedCheck(ctx, s, origin, i)
				}
			}(i)
		}
		wg.Wait()
		for i := start; i < end; i++ {
			if errs[i-start] != nil {
				failures = append(failures, errs[i-start])
				continue
			}
			if sources[i].Kind != "inline" {
				if err := a.emit(sources[i]); err != nil {
					return err
				}
			}
		}
	}
	return errors.Join(failures...)
}

func (a *application) cachedCheck(ctx context.Context, s source, origin *url.URL, index int) error {
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
			return entry.err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	entry := &checkEntry{done: make(chan struct{})}
	a.checks[key] = entry
	a.cacheMu.Unlock()
	entry.err = a.checkScript(ctx, s, origin, index)
	close(entry.done)
	return entry.err
}

func (a *application) checkScript(ctx context.Context, s source, origin *url.URL, index int) error {
	u, _ := parseHTTPURL(s.URL)
	resp, err := request(ctx, a.client, a.c, u, origin)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotModified {
		return fmt.Errorf("script %s: HTTP %d", s.URL, resp.StatusCode)
	}
	if a.c.save {
		if resp.StatusCode == http.StatusNotModified {
			return fmt.Errorf("script %s: HTTP 304 has no body to save", s.URL)
		}
		_, err = saveScript(s, resp.Body, index)
		return err
	}
	// Bound draining so that a large script does not monopolize a worker.
	_, err = io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))
	return err
}

func (a *application) emit(s source) error {
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
