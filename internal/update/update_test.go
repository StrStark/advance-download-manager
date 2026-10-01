package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"0.2.0", "0.1.9", true}, {"v1.0.0", "0.9.9", true}, {"0.2.0", "0.2.0", false},
		{"0.2.0", "0.2.0-beta.1", true}, {"0.2.0-beta.2", "0.2.0-beta.1", true},
		{"0.1.0", "0.0.0-dev", true}, {"0.1.0", "0.2.0", false}, {"0.10.0", "0.9.0", true},
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%s, %s) = %v", c.a, c.b, got)
		}
	}
}

func TestPickAsset(t *testing.T) {
	v := "0.2.0"
	r := &Release{Version: v}
	for _, n := range []string{"adm_0.2.0_linux_amd64.deb", "adm_0.2.0_linux_amd64", "adm_0.2.0_linux_arm64", "adm-server_0.2.0_linux_amd64",
		"adm_0.2.0_windows_amd64_setup.exe", "adm_0.2.0_macos_universal.dmg", "adm_0.2.0_android_arm64-v8a.apk",
		"adm_0.2.0_android_universal.apk", "SHA256SUMS"} {
		r.Assets = append(r.Assets, Asset{Name: n})
	}
	arch := runtime.GOARCH
	check := func(k Kind, want string) {
		a := PickAsset(r, k)
		got := ""
		if a != nil {
			got = a.Name
		}
		if got != want {
			t.Errorf("%s: got %q want %q", k, got, want)
		}
	}
	if arch == "amd64" {
		check(Deb, "adm_0.2.0_linux_amd64.deb")
		check(Portable, "adm_0.2.0_linux_amd64")
		check(Server, "adm-server_0.2.0_linux_amd64")
		check(Windows, "adm_0.2.0_windows_amd64_setup.exe")
		check(Android, "adm_0.2.0_android_universal.apk") // x86_64 build missing → universal
	}
	check(MacOS, "adm_0.2.0_macos_universal.dmg")
	check(Container, "")
}

func TestDownloadVerifiesChecksum(t *testing.T) {
	payload := []byte("new ADM binary")
	sum := sha256.Sum256(payload)
	good := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/SHA256SUMS":
			s := hex.EncodeToString(sum[:])
			if !good {
				s = "00" + s[2:]
			}
			fmt.Fprintf(w, "%s  adm_1.0.0_linux_amd64\n", s)
		default:
			w.Write(payload)
		}
	}))
	defer srv.Close()
	r := &Release{Version: "1.0.0", Assets: []Asset{{Name: "adm_1.0.0_linux_amd64", URL: srv.URL + "/bin"}, {Name: "SHA256SUMS", URL: srv.URL + "/SHA256SUMS"}}}
	dir := t.TempDir()
	var lastDone int64
	p, err := Download(context.Background(), srv.Client(), r, &r.Assets[0], dir, func(d, _ int64) { lastDone = d })
	if err != nil || lastDone != int64(len(payload)) {
		t.Fatalf("download: %v (%d bytes)", err, lastDone)
	}
	if b, _ := os.ReadFile(p); string(b) != string(payload) {
		t.Fatal("content")
	}
	good = false
	if _, err := Download(context.Background(), srv.Client(), r, &r.Assets[0], dir, nil); err == nil {
		t.Fatal("corrupted download should fail")
	}
}

func TestLatestParsesGitHub(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tag_name":"v0.3.1","name":"ADM 0.3.1","body":"notes","html_url":"https://x/r","assets":[{"name":"SHA256SUMS","browser_download_url":"https://x/s","size":10}]}`)
	}))
	defer srv.Close()
	c := srv.Client()
	c.Transport = rewrite{srv.URL}
	r, err := Latest(context.Background(), c)
	if err != nil || r.Version != "0.3.1" || len(r.Assets) != 1 {
		t.Fatalf("%+v %v", r, err)
	}
}

type rewrite struct{ base string }

func (rw rewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	u := *req.URL
	nu, _ := http.NewRequest(req.Method, rw.base+u.Path, nil)
	return http.DefaultTransport.RoundTrip(nu)
}
