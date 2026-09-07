package main

import (
	"runtime/debug"
	"strings"
)

var version = "dev"
var commit = "unknown"

func currentVersion() string {
	info, _ := debug.ReadBuildInfo()
	return describeBuild(info, version, commit)
}

func describeBuild(info *debug.BuildInfo, versionOverride, commitOverride string) string {
	v, revision, dirty := versionOverride, commitOverride, false
	if v == "" {
		v = "dev"
	}
	if revision == "unknown" {
		revision = ""
	}
	if info != nil {
		if v == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
			v = info.Main.Version
		}
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && revision == "" {
				revision = setting.Value
			}
			if setting.Key == "vcs.modified" && setting.Value == "true" {
				dirty = true
			}
		}
		if revision == "" {
			// A Go pseudo-version embeds its source revision; a release tag does not.
			parts := strings.Split(info.Main.Version, "-")
			if len(parts) >= 3 {
				date, hash := parts[len(parts)-2], parts[len(parts)-1]
				if len(date) == 14 && strings.Trim(date, "0123456789") == "" && len(hash) == 12 && strings.Trim(hash, "0123456789abcdef") == "" {
					revision = hash
				}
			}
		}
	}
	result := "getJS " + v
	if revision != "" {
		result += " (" + revision[:min(12, len(revision))] + ")"
	}
	if dirty {
		result += " [modified]"
	}
	return result
}
