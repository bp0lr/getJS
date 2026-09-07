package main

import (
	"io"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

type source struct {
	Page     string `json:"page"`
	URL      string `json:"url,omitempty"`
	Raw      string `json:"reference,omitempty"`
	Kind     string `json:"kind"`
	Inline   string `json:"-"`
	Status   int    `json:"status,omitempty"`
	FinalURL string `json:"final_url,omitempty"`
	Path     string `json:"path,omitempty"`
	Size     int64  `json:"size,omitempty"`
	Error    string `json:"error,omitempty"`
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
	doc.Find("script, link[href]").Each(func(_ int, s *goquery.Selection) {
		if goquery.NodeName(s) == "link" {
			kind := preloadKind(s.AttrOr("rel", ""), s.AttrOr("as", ""))
			if kind == "" {
				return
			}
			raw := strings.TrimSpace(s.AttrOr("href", ""))
			if raw == "" {
				return
			}
			u, err := resolveReference(base, raw)
			if err == nil {
				sources = append(sources, source{Page: page, URL: u.String(), Raw: raw, Kind: kind})
			}
			return
		}
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

func preloadKind(rel, as string) string {
	as = strings.ToLower(strings.TrimSpace(as))
	tokens := strings.Fields(strings.ToLower(rel))
	for _, token := range tokens {
		if token == "modulepreload" {
			switch as {
			case "", "script", "worker", "sharedworker", "serviceworker", "audioworklet", "paintworklet":
				return "modulepreload"
			}
		}
	}
	if as == "script" {
		for _, token := range tokens {
			if token == "preload" {
				return "preload"
			}
		}
	}
	return ""
}

func resolveReference(base *url.URL, raw string) (*url.URL, error) {
	ref, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	return parseHTTPURL(base.ResolveReference(ref).String())
}
