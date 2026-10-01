// Package update checks GitHub for new ADM releases and installs them.
//
// Release assets follow a fixed naming scheme (see .github/workflows/release.yml):
//
//	adm_<ver>_linux_amd64.deb        Debian/Ubuntu package
//	adm_<ver>_linux_amd64            portable Linux binary
//	adm_<ver>_windows_amd64_setup.exe
//	adm_<ver>_macos_universal.dmg
//	adm_<ver>_android_<abi>.apk      arm64-v8a | armeabi-v7a | x86_64 | universal
//	adm-server_<ver>_linux_<arch>    headless server
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/StrStark/advance-download-manager/internal/version"
)

// Release is a published GitHub release.
type Release struct {
	Version     string  `json:"version"` // without the leading "v"
	Name        string  `json:"name"`
	Notes       string  `json:"notes"`
	URL         string  `json:"url"` // release page
	PublishedAt string  `json:"publishedAt"`
	Assets      []Asset `json:"assets"`
}

// Asset is one downloadable file of a release.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Size int64  `json:"size"`
}

// Latest fetches the newest release from GitHub.
func Latest(ctx context.Context, c *http.Client) (*Release, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+version.Repo+"/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ADM/"+version.Version)
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach GitHub: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("no releases published yet")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub returned %s", resp.Status)
	}
	var gh struct {
		TagName     string `json:"tag_name"`
		Name        string `json:"name"`
		Body        string `json:"body"`
		HTMLURL     string `json:"html_url"`
		PublishedAt string `json:"published_at"`
		Assets      []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
			Size int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&gh); err != nil {
		return nil, err
	}
	r := &Release{Version: strings.TrimPrefix(gh.TagName, "v"), Name: gh.Name, Notes: gh.Body, URL: gh.HTMLURL, PublishedAt: gh.PublishedAt}
	for _, a := range gh.Assets {
		r.Assets = append(r.Assets, Asset{Name: a.Name, URL: a.URL, Size: a.Size})
	}
	return r, nil
}

// Newer reports whether version a is newer than b (semver, "v" optional;
// pre-releases sort before the release; dev builds are never newer).
func Newer(a, b string) bool {
	return compare(a, b) > 0
}

func compare(a, b string) int {
	pa, preA := parse(a)
	pb, preB := parse(b)
	for i := range 3 {
		if pa[i] != pb[i] {
			if pa[i] > pb[i] {
				return 1
			}
			return -1
		}
	}
	switch {
	case preA == preB:
		return 0
	case preA == "":
		return 1
	case preB == "":
		return -1
	case preA > preB:
		return 1
	}
	return -1
}

func parse(v string) ([3]int, string) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	v, pre, _ := strings.Cut(v, "-")
	var out [3]int
	for i, p := range strings.SplitN(v, ".", 3) {
		out[i], _ = strconv.Atoi(p)
	}
	return out, pre
}

// Kind is how this copy of ADM was installed, which decides how to update it.
type Kind string

const (
	Deb       Kind = "deb"       // /usr/bin/adm from the .deb: apt installs the new package
	Portable  Kind = "portable"  // a loose binary: replaced in place
	Windows   Kind = "windows"   // the setup.exe installer
	MacOS     Kind = "macos"     // ADM.app: replaced from the .dmg
	Android   Kind = "android"   // the APK installer
	Container Kind = "container" // Docker: pull the new image
	Server    Kind = "server"    // headless binary outside Docker
)

// Detect works out the install kind for a shell ("desktop", "server", "android").
func Detect(shell string) Kind {
	switch shell {
	case "android":
		return Android
	case "server":
		if _, err := os.Stat("/.dockerenv"); err == nil || os.Getenv("ADM_DATA_DIR") == "/data" {
			return Container
		}
		return Server
	}
	switch runtime.GOOS {
	case "windows":
		return Windows
	case "darwin":
		return MacOS
	}
	exe, _ := os.Executable()
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	if strings.HasPrefix(exe, "/usr/bin/") || strings.HasPrefix(exe, "/usr/lib/") {
		return Deb
	}
	return Portable
}

// PickAsset returns the file to download for this platform, or nil.
func PickAsset(r *Release, k Kind) *Asset {
	arch := runtime.GOARCH
	var want []string
	switch k {
	case Deb:
		want = []string{"_linux_" + arch + ".deb"}
	case Portable:
		want = []string{"_linux_" + arch}
	case Windows:
		want = []string{"_windows_" + arch + "_setup.exe"}
	case MacOS:
		want = []string{"_macos_universal.dmg"}
	case Android:
		abi := map[string]string{"arm64": "arm64-v8a", "arm": "armeabi-v7a", "amd64": "x86_64"}[arch]
		want = []string{"_android_" + abi + ".apk", "_android_universal.apk"}
	case Server:
		want = []string{"_linux_" + arch}
	}
	for _, suffix := range want {
		for i, a := range r.Assets {
			isServer := strings.HasPrefix(a.Name, "adm-server_")
			if strings.HasSuffix(a.Name, suffix) && isServer == (k == Server) {
				return &r.Assets[i]
			}
		}
	}
	return nil
}

// Download fetches the asset into dir, reporting progress, and verifies it
// against the release's SHA256SUMS when present.
func Download(ctx context.Context, c *http.Client, r *Release, a *Asset, dir string, progress func(done, total int64)) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, a.Name)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	req.Header.Set("User-Agent", "ADM/"+version.Version)
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: %s", resp.Status)
	}
	f, err := os.Create(path + ".part")
	if err != nil {
		return "", err
	}
	h := sha256.New()
	total := resp.ContentLength
	var done int64
	buf := make([]byte, 128<<10)
	last := time.Now()
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				f.Close()
				return "", err
			}
			h.Write(buf[:n])
			done += int64(n)
			if progress != nil && time.Since(last) > 200*time.Millisecond {
				progress(done, total)
				last = time.Now()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			return "", rerr
		}
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if progress != nil {
		progress(done, total)
	}
	if want := expectedSum(ctx, c, r, a.Name); want != "" && want != hex.EncodeToString(h.Sum(nil)) {
		os.Remove(path + ".part")
		return "", errors.New("downloaded update is corrupted (checksum mismatch)")
	}
	return path, os.Rename(path+".part", path)
}

// expectedSum reads the asset's checksum from SHA256SUMS, if published.
func expectedSum(ctx context.Context, c *http.Client, r *Release, name string) string {
	for _, a := range r.Assets {
		if a.Name != "SHA256SUMS" {
			continue
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
		resp, err := c.Do(req)
		if err != nil {
			return ""
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		for _, line := range strings.Split(string(b), "\n") {
			f := strings.Fields(line)
			if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
				return strings.ToLower(f[0])
			}
		}
	}
	return ""
}
