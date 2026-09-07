package main

import (
	"net/url"
	"strings"
	"testing"
)

func TestResolveReference(t *testing.T) {
	base, _ := url.Parse("https://example.test/a/page.html?old=1")
	tests := []struct{ raw, want string }{
		{"/", "https://example.test/"},
		{"app.js", "https://example.test/a/app.js"},
		{"../app.js", "https://example.test/app.js"},
		{"//cdn.test/a.js", "https://cdn.test/a.js"},
		{"?v=2#section", "https://example.test/a/page.html?v=2"},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := resolveReference(base, tt.raw)
			if err != nil || got.String() != tt.want {
				t.Fatalf("got %v, %v; want %s", got, err, tt.want)
			}
		})
	}
	for _, raw := range []string{"javascript:alert(1)", "data:text/javascript,x", "https://user:pass@example.test/a.js", "%zz"} {
		if _, err := resolveReference(base, raw); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
}

func TestExtractBaseAndAttributePriority(t *testing.T) {
	u, _ := url.Parse("https://example.test/redirected/page")
	html := "<base href='data:invalid'><base href='/assets/'><base href='/ignored/'>" +
		"<script src='one.js' data-src='two.js'>fallback</script>" +
		"<script data-src='../lazy.js'>fallback</script><script> inline() </script>" +
		"<script src='data:text/javascript,x'></script>"
	got, err := extract(strings.NewReader(html), "https://example.test/original", u)
	if err != nil || len(got) != 3 {
		t.Fatalf("got %#v, %v", got, err)
	}
	if got[0].URL != "https://example.test/assets/one.js" || got[1].URL != "https://example.test/lazy.js" {
		t.Fatalf("URLs: %#v", got)
	}
	if got[2].Kind != "inline" || got[2].Inline != " inline() " {
		t.Fatalf("inline: %#v", got[2])
	}
	if got[0].Page != "https://example.test/original" {
		t.Fatal("lost input page provenance")
	}
}
