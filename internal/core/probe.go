package core

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// ProbeResult describes what the server told us about a resource.
type ProbeResult struct {
	URL          string `json:"url"`
	FinalURL     string `json:"finalUrl"`
	Filename     string `json:"filename"`
	Size         int64  `json:"size"` // -1 when unknown
	Resumable    bool   `json:"resumable"`
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"lastModified,omitempty"`
	ContentType  string `json:"contentType,omitempty"`
	Category     string `json:"category"`
	Error        string `json:"error,omitempty"`
}

// Probe discovers size, range support and filename. It tries HEAD first and
// falls back to a one-byte ranged GET, which also confirms range support.
func Probe(ctx context.Context, client *http.Client, rawURL string, headers map[string]string) (ProbeResult, error) {
	res := ProbeResult{URL: rawURL, Size: -1}
	if _, err := url.ParseRequestURI(rawURL); err != nil {
		return res, fmt.Errorf("invalid URL")
	}

	resp, err := doProbe(ctx, client, http.MethodHead, rawURL, headers, false)
	if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 && resp.ContentLength > 0 {
		fill(&res, resp)
		res.Resumable = strings.EqualFold(resp.Header.Get("Accept-Ranges"), "bytes")
		resp.Body.Close()
		if res.Resumable {
			return finish(res), nil
		}
	} else if resp != nil {
		resp.Body.Close()
	}

	// Ranged GET: authoritative for range support.
	resp, err = doProbe(ctx, client, http.MethodGet, rawURL, headers, true)
	if err != nil {
		return res, err
	}
	defer resp.Body.Close()
	io.CopyN(io.Discard, resp.Body, 1)
	switch {
	case resp.StatusCode == http.StatusPartialContent:
		fill(&res, resp)
		res.Resumable = true
		if total := parseContentRangeTotal(resp.Header.Get("Content-Range")); total > 0 {
			res.Size = total
		}
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		fill(&res, resp)
		res.Resumable = false
		res.Size = resp.ContentLength
		if res.Size <= 0 {
			res.Size = -1
		}
	default:
		return res, fmt.Errorf("server returned %s", resp.Status)
	}
	return finish(res), nil
}

func doProbe(ctx context.Context, client *http.Client, method, u string, headers map[string]string, ranged bool) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return nil, err
	}
	applyHeaders(req, headers)
	if ranged {
		req.Header.Set("Range", "bytes=0-0")
	}
	return client.Do(req)
}

func applyHeaders(req *http.Request, headers map[string]string) {
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", DefaultUserAgent)
	}
	// Transparent compression breaks byte ranges and lengths.
	req.Header.Set("Accept-Encoding", "identity")
}

func fill(res *ProbeResult, resp *http.Response) {
	res.FinalURL = resp.Request.URL.String()
	res.Size = resp.ContentLength
	if res.Size <= 0 {
		res.Size = -1
	}
	res.ETag = resp.Header.Get("ETag")
	res.LastModified = resp.Header.Get("Last-Modified")
	res.ContentType = resp.Header.Get("Content-Type")
	res.Filename = FilenameFromResponse(resp.Header.Get("Content-Disposition"), resp.Request.URL, res.ContentType)
}

func finish(res ProbeResult) ProbeResult {
	if res.Filename == "" {
		u, _ := url.Parse(res.URL)
		res.Filename = FilenameFromResponse("", u, res.ContentType)
	}
	res.Category = CategoryFor(res.Filename)
	return res
}

func parseContentRangeTotal(v string) int64 {
	// bytes 0-0/12345
	i := strings.LastIndexByte(v, '/')
	if i < 0 {
		return -1
	}
	n, err := strconv.ParseInt(strings.TrimSpace(v[i+1:]), 10, 64)
	if err != nil {
		return -1
	}
	return n
}

// FilenameFromResponse picks a filename from Content-Disposition, then the URL
// path, then a generic name with an extension guessed from the MIME type.
func FilenameFromResponse(disposition string, u *url.URL, contentType string) string {
	if disposition != "" {
		if _, params, err := mime.ParseMediaType(disposition); err == nil {
			if name := params["filename"]; name != "" { // mime decodes filename* into filename
				return SanitizeFilename(name)
			}
		}
	}
	if u != nil {
		if base := path.Base(u.Path); base != "" && base != "/" && base != "." {
			if dec, err := url.PathUnescape(base); err == nil {
				base = dec
			}
			return SanitizeFilename(base)
		}
	}
	name := "download"
	if mt, _, err := mime.ParseMediaType(contentType); err == nil {
		if exts, _ := mime.ExtensionsByType(mt); len(exts) > 0 {
			name += exts[0]
		}
	}
	if u != nil && u.Host != "" {
		name = u.Hostname() + "-" + name
	}
	return name
}

// SanitizeFilename makes a name safe on Linux, macOS and Windows: no path
// separators, control or reserved characters, reserved device names, or
// trailing dots and spaces.
func SanitizeFilename(name string) string {
	name = strings.Map(func(r rune) rune {
		switch {
		case r < 32, r == 0x7f:
			return '_'
		case strings.ContainsRune(`/\<>:"|?*`, r):
			return '_'
		}
		return r
	}, name)
	name = strings.TrimRight(strings.TrimSpace(name), ". ")
	if name == "" || name == "." || name == ".." {
		return "download"
	}
	base := strings.ToUpper(strings.TrimSuffix(name, filepath.Ext(name)))
	switch base {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		name = "_" + name
	}
	if len(name) > 240 {
		ext := filepath.Ext(name)
		if len(ext) > 16 {
			ext = ""
		}
		name = name[:240-len(ext)] + ext
	}
	return name
}

var categoryExts = map[string][]string{
	"video":     {".mp4", ".mkv", ".webm", ".avi", ".mov", ".m4v", ".flv", ".wmv", ".ts", ".m2ts"},
	"music":     {".mp3", ".flac", ".ogg", ".opus", ".wav", ".m4a", ".aac", ".wma"},
	"archives":  {".zip", ".rar", ".7z", ".tar", ".gz", ".xz", ".zst", ".bz2", ".tgz", ".txz", ".iso", ".img", ".dmg"},
	"documents": {".pdf", ".doc", ".docx", ".odt", ".xls", ".xlsx", ".ods", ".ppt", ".pptx", ".epub", ".txt", ".md", ".csv"},
	"images":    {".jpg", ".jpeg", ".png", ".gif", ".webp", ".svg", ".avif", ".heic", ".bmp", ".tiff"},
	"programs":  {".deb", ".rpm", ".appimage", ".flatpak", ".snap", ".sh", ".run", ".bin", ".exe", ".msi", ".apk", ".jar"},
}

// CategoryFor classifies a filename by extension.
func CategoryFor(name string) string {
	lower := strings.ToLower(name)
	for cat, exts := range categoryExts {
		for _, e := range exts {
			if strings.HasSuffix(lower, e) {
				return cat
			}
		}
	}
	return "other"
}
