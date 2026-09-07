package main

import (
	"context"
	"fmt"
	"io"
	"os"
)

func (a *application) processHTMLFile(ctx context.Context, page string) error {
	f, err := os.Open(a.c.htmlFile)
	if err != nil {
		return err
	}
	defer f.Close()
	if a.c.verbose {
		fmt.Fprintln(a.errOut, "getJS: reading local HTML", a.c.htmlFile)
	}
	body := &io.LimitedReader{R: f, N: a.c.maxBody + 1}
	base, _ := parseHTTPURL(a.c.baseURL)
	sources, err := extract(body, page, base)
	if err != nil {
		return err
	}
	if body.N == 0 {
		return fmt.Errorf("HTML file exceeds --max-body-size (%d bytes)", a.c.maxBody)
	}
	return a.processSources(ctx, sources, base)
}
