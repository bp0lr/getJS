package main

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"
)

func safeName(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("-_.", r) {
			return r
		}
		return '_'
	}, s)
	s = strings.Trim(s, ". ")
	if s == "" {
		return "script"
	}
	stem := strings.ToUpper(strings.SplitN(s, ".", 2)[0])
	if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" ||
		(len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '0' && stem[3] <= '9') {
		s = "_" + s
	}
	if len(s) > 100 {
		s = string([]rune(s)[:min(40, len([]rune(s)))])
	}
	return s
}

func saveScript(s source, r io.Reader, index int) (string, error) {
	page, _ := url.Parse(s.Page)
	dir := filepath.Join("download", safeName(page.Host))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("inline-%d.js", index+1)
	if s.Kind != "inline" {
		u, _ := url.Parse(s.URL)
		name = safeName(path.Base(u.Path))
		if path.Ext(name) == "" {
			name += ".js"
		}
	}
	for i := 0; ; i++ {
		candidate := name
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d%s", strings.TrimSuffix(name, path.Ext(name)), i, path.Ext(name))
		}
		dest := filepath.Join(dir, candidate)
		f, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, copyErr := io.Copy(f, r)
		err = errors.Join(copyErr, f.Close())
		if err != nil {
			return "", errors.Join(err, os.Remove(dest))
		}
		return dest, nil
	}
}
