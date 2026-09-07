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
	"strings"
)

type application struct {
	c           config
	client      *http.Client
	out, errOut io.Writer
	records     int
	writeErr    error
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
	pages, err := readInputs(c, stdin)
	if err != nil {
		fmt.Fprintln(stderr, "getJS:", err)
		return 1
	}
	if len(pages) == 0 {
		fmt.Fprintln(stderr, "getJS: no page URLs supplied")
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
	a := application{c: c, client: newHTTPClient(c), out: writer, errOut: stderr}
	defer a.client.CloseIdleConnections()
	failed, succeeded := 0, 0
	for _, page := range pages {
		if ctx.Err() != nil {
			return 130
		}
		if err := a.processPage(ctx, page); err != nil {
			failed++
			fmt.Fprintln(stderr, "getJS:", err)
		} else {
			succeeded++
		}
		if a.writeErr != nil {
			return 1
		}
	}
	if ctx.Err() != nil {
		return 130
	}
	if failed > 0 {
		if succeeded > 0 || a.records > 0 {
			return 2
		}
		return 1
	}
	return 0
}

func readInputs(c config, stdin io.Reader) ([]string, error) {
	var pages []string
	read := func(r io.Reader) error {
		s := bufio.NewScanner(r)
		s.Buffer(make([]byte, 4096), 1024*1024)
		for s.Scan() {
			if line := strings.TrimSpace(s.Text()); line != "" {
				pages = append(pages, line)
			}
		}
		return s.Err()
	}
	if stdin != nil {
		if err := read(stdin); err != nil {
			return nil, fmt.Errorf("stdin: %w", err)
		}
	}
	if c.input != "" {
		f, err := os.Open(c.input)
		if err != nil {
			return nil, fmt.Errorf("input: %w", err)
		}
		err = read(f)
		closeErr := f.Close()
		if err := errors.Join(err, closeErr); err != nil {
			return nil, fmt.Errorf("input: %w", err)
		}
	}
	if c.url != "" {
		pages = append(pages, strings.TrimSpace(c.url))
	}
	return pages, nil
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
	if err != nil {
		return err
	}
	return a.processSources(ctx, sources, resp.Request.URL)
}

func (a *application) processSources(ctx context.Context, sources []source, origin *url.URL) error {
	var failures []error
	for i, s := range sources {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if s.Kind == "inline" {
			if a.c.save {
				if _, err := saveScript(s, strings.NewReader(s.Inline), i); err != nil {
					failures = append(failures, err)
				}
			}
			continue
		}
		if a.c.resolve || a.c.save {
			if err := a.checkScript(ctx, s, origin, i); err != nil {
				failures = append(failures, err)
				continue
			}
		}
		if err := a.emit(s); err != nil {
			return err
		}
	}
	return errors.Join(failures...)
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
	return nil
}
