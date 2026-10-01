package admmobile

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type recorder struct {
	mu       sync.Mutex
	active   []int
	progress int
	done     []string
}

func (r *recorder) OnActiveChanged(n int) { r.mu.Lock(); r.active = append(r.active, n); r.mu.Unlock() }
func (r *recorder) OnProgress(int, int64, int64, int64) {
	r.mu.Lock()
	r.progress++
	r.mu.Unlock()
}
func (r *recorder) OnMultiLinkChanged(bool) {}
func (r *recorder) OnInstallUpdate(string)  {}

func (r *recorder) OnDownloadComplete(name, path string, inBatch bool) {
	r.mu.Lock()
	r.done = append(r.done, path)
	r.mu.Unlock()
}

func TestAndroidEntryPoint(t *testing.T) {
	payload := bytes.Repeat([]byte("adm!"), 1<<20) // 4 MiB
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond) // keep it running long enough to observe
		http.ServeContent(w, r, "file.bin", time.Now(), bytes.NewReader(payload))
	}))
	defer src.Close()

	data, dl := t.TempDir(), t.TempDir()
	url, err := Start(data, dl)
	if err != nil {
		t.Fatal(err)
	}
	defer Stop()
	if again, _ := Start(data, dl); again != url {
		t.Fatal("Start is not idempotent")
	}
	rec := &recorder{}
	SetListener(rec)

	// The token URL authorizes the WebView (cookie), plain requests are refused.
	base := url[:strings.Index(url, "/?token=")]
	if r, _ := http.Get(base + "/api/ping"); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", r.StatusCode)
	}
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	if r, err := c.Get(url); err != nil || r.StatusCode != http.StatusOK {
		t.Fatalf("token URL: %v", err)
	}

	// A share before the UI subscribes is queued for PendingURLs.
	OpenURLs("check this out " + src.URL + "/file.bin")
	resp, err := c.Post(base+"/api/call/PendingURLs", "application/json", strings.NewReader("[]"))
	if err != nil {
		t.Fatal(err)
	}
	var pending struct{ Result []string }
	json.NewDecoder(resp.Body).Decode(&pending)
	if len(pending.Result) != 1 || pending.Result[0] != src.URL+"/file.bin" {
		t.Fatalf("pending = %v", pending.Result)
	}

	// Add it through the API like the UI would.
	body := `[{"url":"` + src.URL + `/file.bin","connections":4}]`
	if r, err := c.Post(base+"/api/call/AddDownload", "application/json", strings.NewReader(body)); err != nil || r.StatusCode != 200 {
		t.Fatalf("AddDownload: %v", err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for {
		rec.mu.Lock()
		n := len(rec.done)
		rec.mu.Unlock()
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("download did not finish; active=%v", rec.active)
		}
		time.Sleep(50 * time.Millisecond)
	}
	got, err := os.ReadFile(filepath.Join(dl, "file.bin"))
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("file mismatch: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.active) < 2 || rec.active[len(rec.active)-1] != 0 || !contains(rec.active, 1) {
		t.Fatalf("active transitions = %v, want to include 1 and end at 0", rec.active)
	}
	if rec.done[0] != filepath.Join(dl, "file.bin") {
		t.Fatalf("done path = %s", rec.done[0])
	}
}

func contains(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
