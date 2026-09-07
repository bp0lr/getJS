package main

import (
	"crypto/rand"
	"crypto/sha256"
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

type savedFile struct {
	Path   string
	Size   int64
	SHA256 string
}

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

func shortHash(s string) string { sum := sha256.Sum256([]byte(s)); return fmt.Sprintf("%x", sum[:8]) }

func saveScript(c config, s source, r io.Reader, index int) (saved savedFile, err error) {
	page, _ := url.Parse(s.Page)
	host := "local"
	if page != nil && page.Host != "" {
		host = safeName(page.Host)
	}
	dir := filepath.Join(host, shortHash(s.Page))
	if err := os.MkdirAll(c.outputDir, 0755); err != nil {
		return saved, err
	}
	root, err := os.OpenRoot(c.outputDir)
	if err != nil {
		return saved, err
	}
	defer root.Close()
	if err := root.MkdirAll(dir, 0755); err != nil {
		return saved, err
	}

	position := s.Position
	if position == 0 {
		position = index + 1
	}
	name := fmt.Sprintf("inline-%d.js", position)
	if s.Kind != "inline" {
		u, _ := url.Parse(s.URL)
		base := safeName(path.Base(u.Path))
		ext := path.Ext(base)
		if ext == "" {
			ext = ".js"
		}
		name = strings.TrimSuffix(base, path.Ext(base)) + "-" + shortHash(s.URL) + ext
	}
	temp := filepath.Join(dir, ".getjs-"+rand.Text()+".tmp")
	f, err := root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return saved, err
	}
	defer func() {
		if cleanupErr := root.Remove(temp); cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
			err = errors.Join(err, cleanupErr)
		}
	}()
	hash := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(f, hash), io.LimitReader(r, c.maxBody+1))
	if n > c.maxBody {
		copyErr = errors.Join(copyErr, fmt.Errorf("script exceeds --max-body-size (%d bytes)", c.maxBody))
	}
	if copyErr == nil {
		copyErr = f.Sync()
	}
	if err := errors.Join(copyErr, f.Close()); err != nil {
		return saved, err
	}
	for i := 0; ; i++ {
		candidate := name
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d%s", strings.TrimSuffix(name, path.Ext(name)), i, path.Ext(name))
		}
		dest := filepath.Join(dir, candidate)
		err := publishDownload(root, temp, dest, root.Link)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return saved, fmt.Errorf("publish download: %w", err)
		}
		return savedFile{Path: filepath.Join(c.outputDir, dest), Size: n, SHA256: fmt.Sprintf("%x", hash.Sum(nil))}, nil
	}
}

// Prefer atomic publication. Exclusive copying also works on filesystems without
// hard links; the destination is visible during the copy, but never overwritten.
func publishDownload(root *os.Root, temp, dest string, link func(string, string) error) (err error) {
	if err = link(temp, dest); err == nil || errors.Is(err, os.ErrExist) {
		return err
	}
	in, err := root.Open(temp)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := root.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, root.Remove(dest))
		}
	}()
	_, copyErr := io.Copy(out, in)
	if copyErr == nil {
		copyErr = out.Sync()
	}
	return errors.Join(copyErr, out.Close())
}
