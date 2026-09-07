package main

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMetadataPreservesPresenceAndIndividualTags(t *testing.T) {
	u, _ := url.Parse("https://example.test")
	got, err := extract(strings.NewReader(
		"<script src='/same.js' type='module' async='false' defer crossorigin integrity='sha256-example'></script>"+
			"<script src='/same.js' nomodule></script>"+
			"<link rel='modulepreload' as='script' href='/same.js' crossorigin='use-credentials'>"), u.String(), u)
	if err != nil || len(got) != 3 {
		t.Fatalf("%#v %v", got, err)
	}
	a := got[0].Metadata
	if a.Type != "module" || !a.Async || !a.Defer || a.CrossOrigin == nil || *a.CrossOrigin != "" || a.Integrity != "sha256-example" {
		t.Fatalf("%#v", a)
	}
	if got[1].Metadata.Async || got[1].Metadata.CrossOrigin != nil || !got[1].Metadata.NoModule {
		t.Fatalf("%#v", got[1].Metadata)
	}
	if got[2].Metadata.Rel != "modulepreload" || got[2].Metadata.As != "script" || *got[2].Metadata.CrossOrigin != "use-credentials" {
		t.Fatalf("%#v", got[2].Metadata)
	}
	data, err := json.Marshal(got)
	if err != nil || !strings.Contains(string(data), "\"crossorigin\":\"\"") {
		t.Fatalf("%s %v", data, err)
	}
}

func TestJSONLMetadataAndStableInlinePosition(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "page.html")
	html := "<script src='https://other.test/a.js'></script><script async>inline()</script>"
	if err := os.WriteFile(file, []byte(html), 0600); err != nil {
		t.Fatal(err)
	}
	code, out, err := invoke("--html-file", file, "--base-url", "https://example.test", "--same-origin", "--jsonl", "--save", "--output-dir", filepath.Join(dir, "saved"))
	items := decodeSources(t, out)
	if code != 0 || len(items) != 1 || items[0].Metadata == nil || items[0].Metadata.Index != 2 || !items[0].Metadata.Async || filepath.Base(items[0].Path) != "inline-2.js" {
		t.Fatalf("%d %#v %s", code, items, err)
	}
}
