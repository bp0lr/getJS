package main

import (
	"io"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

type source struct {
	Page   string
	URL    string
	Raw    string
	Kind   string
	Inline string
}

func extract(r io.Reader, page string, finalURL *url.URL) ([]source, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, err
	}
	base := finalURL
	doc.Find("base[href]").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		href, _ := s.Attr("href")
		u, err := resolveReference(finalURL, href)
		if err != nil {
			return true
		}
		base = u
		return false
	})
	var sources []source
	doc.Find("script").Each(func(_ int, s *goquery.Selection) {
		raw := strings.TrimSpace(s.AttrOr("src", ""))
		if raw == "" {
			raw = strings.TrimSpace(s.AttrOr("data-src", ""))
		}
		if raw != "" {
			u, err := resolveReference(base, raw)
			if err == nil {
				sources = append(sources, source{Page: page, URL: u.String(), Raw: raw, Kind: "script"})
			}
			return
		}
		if code := s.Text(); strings.TrimSpace(code) != "" {
			sources = append(sources, source{Page: page, Kind: "inline", Inline: code})
		}
	})
	return sources, nil
}

func resolveReference(base *url.URL, raw string) (*url.URL, error) {
	ref, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	return parseHTTPURL(base.ResolveReference(ref).String())
}
