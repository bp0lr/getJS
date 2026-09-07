package main

import (
	"runtime/debug"
	"testing"
)

func TestBuildVersionSources(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		info                  *debug.BuildInfo
		version, commit, want string
	}{
		{"no info", nil, "dev", "unknown", "getJS dev"},
		{"installed tag", &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}}, "dev", "unknown", "getJS v0.1.0"},
		{"installed commit", &debug.BuildInfo{Main: debug.Module{Version: "v0.0.0-20260907154905-aa9186066f82"}}, "dev", "unknown", "getJS v0.0.0-20260907154905-aa9186066f82 (aa9186066f82)"},
		{"working tree", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}, Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "123456789abcde"}, {Key: "vcs.modified", Value: "true"}}}, "dev", "unknown", "getJS dev (123456789abc) [modified]"},
		{"injected release", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}, Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "old"}}}, "v0.1.0", "abcdef123456789", "getJS v0.1.0 (abcdef123456)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := describeBuild(tc.info, tc.version, tc.commit); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
