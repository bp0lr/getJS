package main

import "github.com/PuerkitoBio/goquery"

type scriptMetadata struct {
	Index          int     `json:"index"`
	Type           string  `json:"type,omitempty"`
	Async          bool    `json:"async"`
	Defer          bool    `json:"defer"`
	NoModule       bool    `json:"nomodule,omitempty"`
	Integrity      string  `json:"integrity,omitempty"`
	CrossOrigin    *string `json:"crossorigin,omitempty"`
	ReferrerPolicy string  `json:"referrerpolicy,omitempty"`
	Rel            string  `json:"rel,omitempty"`
	As             string  `json:"as,omitempty"`
}

func readMetadata(s *goquery.Selection, index int) *scriptMetadata {
	_, async := s.Attr("async")
	_, deferAttr := s.Attr("defer")
	_, nomodule := s.Attr("nomodule")
	var crossOrigin *string
	if value, present := s.Attr("crossorigin"); present {
		crossOrigin = &value
	}
	return &scriptMetadata{
		Index:          index,
		Type:           s.AttrOr("type", ""),
		Async:          async,
		Defer:          deferAttr,
		NoModule:       nomodule,
		Integrity:      s.AttrOr("integrity", ""),
		CrossOrigin:    crossOrigin,
		ReferrerPolicy: s.AttrOr("referrerpolicy", ""),
		Rel:            s.AttrOr("rel", ""),
		As:             s.AttrOr("as", ""),
	}
}
