package httpapi

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/StrStark/advance-download-manager/internal/core"
	"github.com/StrStark/advance-download-manager/internal/service"
)

func newTestServer(t *testing.T, cfg Config) *httptest.Server {
	t.Helper()
	hub := NewHub()
	m := core.NewManager(core.NewStore(filepath.Join(t.TempDir(), "state.json")), hub)
	svc := service.New(m)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := svc.Attach(ctx, hub.Emit); err != nil {
		t.Fatal(err)
	}
	assets := fstest.MapFS{"index.html": {Data: []byte("<title>ADM</title>")}}
	srv := httptest.NewServer(Handler(cfg, svc, hub, assets))
	t.Cleanup(srv.Close)
	return srv
}

func TestTokenAuthSetsCookie(t *testing.T) {
	srv := newTestServer(t, Config{Kind: "android", Token: "s3cret"})
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}

	if r, _ := c.Get(srv.URL + "/api/ping"); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no token: got %d", r.StatusCode)
	}
	if r, _ := c.Get(srv.URL + "/?token=wrong"); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong token: got %d", r.StatusCode)
	}
	if r, _ := c.Get(srv.URL + "/?token=s3cret"); r.StatusCode != http.StatusOK {
		t.Fatalf("token: got %d", r.StatusCode)
	}
	// The cookie now authorizes API calls without the query parameter.
	r, err := c.Get(srv.URL + "/api/ping")
	if err != nil || r.StatusCode != http.StatusOK {
		t.Fatalf("cookie: %v %d", err, r.StatusCode)
	}
}

func TestBasicAuth(t *testing.T) {
	srv := newTestServer(t, Config{User: "admin", Pass: "pw"})
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/ping", nil)
	if r, _ := http.DefaultClient.Do(req); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got %d", r.StatusCode)
	}
	req.SetBasicAuth("admin", "pw")
	if r, _ := http.DefaultClient.Do(req); r.StatusCode != http.StatusOK {
		t.Fatalf("got %d", r.StatusCode)
	}
}

func TestConfine(t *testing.T) {
	cfg := Config{DownloadRoot: "/downloads"}
	ok := map[string]string{
		"":                  "",
		"videos":            "/downloads/videos",
		"/downloads/a/b":    "/downloads/a/b",
		"~/music":           "/downloads/music",
		"/downloads/x/../y": "/downloads/y",
	}
	for in, want := range ok {
		got, err := confine(cfg, in)
		if err != nil || got != want {
			t.Errorf("confine(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"/etc", "../x", "/downloads/../etc", "/downloadsX"} {
		if _, err := confine(cfg, bad); err == nil {
			t.Errorf("confine(%q) should fail", bad)
		}
	}
}

func TestDispatchKindAndPending(t *testing.T) {
	pending := []string{"https://example.com/a.zip"}
	srv := newTestServer(t, Config{Kind: "android", PendingURLs: func() []string { p := pending; pending = nil; return p }})
	call := func(m string) string {
		r, err := http.Post(srv.URL+"/api/call/"+m, "application/json", strings.NewReader("[]"))
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		var b strings.Builder
		buf := make([]byte, 512)
		n, _ := r.Body.Read(buf)
		b.Write(buf[:n])
		return strings.TrimSpace(b.String())
	}
	if got := call("Kind"); got != `{"result":"android"}` {
		t.Fatalf("Kind: %s", got)
	}
	if got := call("PendingURLs"); got != `{"result":["https://example.com/a.zip"]}` {
		t.Fatalf("PendingURLs: %s", got)
	}
	if got := call("PendingURLs"); got != `{"result":null}` && got != `{"result":[]}` {
		t.Fatalf("PendingURLs second call: %s", got)
	}
}
