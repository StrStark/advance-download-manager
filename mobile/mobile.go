// Package admmobile is the Android entry point, compiled into an .aar with
// `gomobile bind`. It runs the same engine as the desktop app and serves the
// UI on a private loopback port; the Android WebView loads that URL.
//
// Only gomobile-friendly types (string, int, int64, bool, error, interfaces)
// appear in the exported API. Kotlin sees it as `admmobile.Admmobile`.
package admmobile

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/StrStark/advance-download-manager/internal/batch"
	"github.com/StrStark/advance-download-manager/internal/core"
	"github.com/StrStark/advance-download-manager/internal/httpapi"
	"github.com/StrStark/advance-download-manager/internal/links"
	"github.com/StrStark/advance-download-manager/internal/service"
)

// dist is a copy of frontend/dist, placed here by scripts/build-android.sh
// (go:embed can't reach outside the package directory).
//
//go:embed all:dist
var dist embed.FS

// Listener receives engine events that the Android side acts on
// (foreground service, notifications, media scanning). Calls arrive on
// arbitrary threads.
type Listener interface {
	// OnActiveChanged fires when the number of running downloads changes.
	OnActiveChanged(active int)
	// OnProgress fires a few times per second while downloads run.
	OnProgress(active int, bytesPerSec int64, downloaded int64, total int64)
	// OnDownloadComplete fires for every finished file.
	OnDownloadComplete(filename string, path string, inBatch bool)
	// OnMultiLinkChanged tells the app whether to keep extra networks (e.g.
	// mobile data next to Wi-Fi) connected for multi-link downloads.
	OnMultiLinkChanged(enabled bool)
}

type engine struct {
	m      *core.Manager
	svc    *service.Service
	hub    *httpapi.Hub
	srv    *http.Server
	cancel context.CancelFunc
	url    string

	mu         sync.Mutex
	listener   Listener
	lastActive int
	pending    []string
}

var (
	mu  sync.Mutex
	eng *engine
)

// Start launches the engine and the loopback UI server, and returns the URL
// the WebView should load (it carries a one-time token). Calling it again
// returns the running instance's URL.
func Start(dataDir, downloadDir string) (string, error) {
	mu.Lock()
	defer mu.Unlock()
	if eng != nil {
		return eng.url, nil
	}
	if dataDir == "" || downloadDir == "" {
		return "", errors.New("dataDir and downloadDir are required")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return "", err
	}
	_ = os.MkdirAll(downloadDir, 0o755)
	// The engine reads these for defaults and "~" expansion.
	os.Setenv("ADM_DOWNLOAD_DIR", downloadDir)
	if os.Getenv("HOME") == "" {
		os.Setenv("HOME", dataDir)
	}

	e := &engine{hub: httpapi.NewHub()}
	e.m = core.NewManager(core.NewStore(filepath.Join(dataDir, "state.json")), events{e})
	e.svc = service.New(e.m)
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	if err := e.svc.Attach(ctx, e.hub.Emit); err != nil {
		cancel()
		return "", err
	}

	token, err := randomToken()
	if err != nil {
		cancel()
		return "", err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		cancel()
		return "", err
	}
	assets, _ := fs.Sub(dist, "dist")
	cfg := httpapi.Config{Kind: "android", Token: token, PendingURLs: e.takePending}
	e.srv = &http.Server{Handler: httpapi.Handler(cfg, e.svc, e.hub, assets), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = e.srv.Serve(ln) }()

	e.url = fmt.Sprintf("http://127.0.0.1:%d/?token=%s", ln.Addr().(*net.TCPAddr).Port, token)
	eng = e
	return e.url, nil
}

// Stop pauses running downloads (they resume on next Start), saves state
// and shuts the UI server down.
func Stop() {
	mu.Lock()
	e := eng
	eng = nil
	mu.Unlock()
	if e == nil {
		return
	}
	e.m.Shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = e.srv.Shutdown(ctx)
	e.cancel()
}

// SetListener registers the Android callback (nil to clear).
func SetListener(l Listener) {
	if e := current(); e != nil {
		e.mu.Lock()
		e.listener = l
		e.lastActive = -1 // force an initial OnActiveChanged
		e.mu.Unlock()
		e.notifyActive()
		if l != nil {
			l.OnMultiLinkChanged(e.m.Settings().MultiLink)
		}
	}
}

// SetLinks receives the device's networks from Android as JSON:
// [{"id":"net-123","name":"wlan0","label":"Wi-Fi","kind":"wifi","handle":123,"addrs":["…"]}].
// Go can't enumerate Android networks itself, and binds sockets by handle.
func SetLinks(linksJSON string) error {
	var ls []links.Link
	if err := json.Unmarshal([]byte(linksJSON), &ls); err != nil {
		return err
	}
	links.SetProvided(ls)
	if e := current(); e != nil {
		e.hub.Emit("links", ls)
	}
	return nil
}

// OpenURLs hands shared text (from the Android share sheet) to the UI. Links
// are extracted; if no UI is connected yet they wait until it asks.
func OpenURLs(text string) {
	e := current()
	if e == nil {
		return
	}
	urls := batch.ExtractURLs(text)
	if len(urls) == 0 {
		return
	}
	if e.hub.Clients() > 0 {
		e.hub.Emit(service.EventExternal, urls)
		return
	}
	e.mu.Lock()
	e.pending = append(e.pending, urls...)
	e.mu.Unlock()
}

// PauseAll pauses every running or queued download (notification action).
func PauseAll() {
	if e := current(); e != nil {
		var ids []string
		for _, j := range e.m.Snapshot().Jobs {
			if j.Status.IsActive() || j.Status == core.StatusQueued {
				ids = append(ids, j.ID)
			}
		}
		e.m.Pause(ids)
	}
}

// ResumeAll resumes every paused download.
func ResumeAll() {
	if e := current(); e != nil {
		var ids []string
		for _, j := range e.m.Snapshot().Jobs {
			if j.Status == core.StatusPaused {
				ids = append(ids, j.ID)
			}
		}
		e.m.Resume(ids)
	}
}

// ActiveCount is the number of downloads currently running.
func ActiveCount() int {
	if e := current(); e != nil {
		return e.m.ActiveCount()
	}
	return 0
}

// CompletedPath returns the file path of a finished download, or "".
func CompletedPath(id string) string {
	if e := current(); e != nil {
		if p, err := e.svc.CompletedPath(id); err == nil {
			return p
		}
	}
	return ""
}

// ---------- internals ----------

func current() *engine {
	mu.Lock()
	defer mu.Unlock()
	return eng
}

func (e *engine) takePending() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	p := e.pending
	e.pending = nil
	return p
}

func (e *engine) getListener() Listener {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.listener
}

func (e *engine) notifyActive() {
	n := e.m.ActiveCount()
	e.mu.Lock()
	l := e.listener
	changed := n != e.lastActive
	e.lastActive = n
	e.mu.Unlock()
	if l != nil && changed {
		l.OnActiveChanged(n)
	}
}

// events tees engine events to the web UI and to the Android listener.
type events struct{ e *engine }

func (x events) Emit(name string, data any) {
	e := x.e
	e.hub.Emit(name, data)
	l := e.getListener()
	if l == nil {
		return
	}
	switch name {
	case core.EventProgress:
		if list, ok := data.([]core.Progress); ok {
			var speed float64
			var done, total int64
			for _, p := range list {
				speed += p.Speed
				done += p.Downloaded
				if p.Size > 0 {
					total += p.Size
				}
			}
			l.OnProgress(len(list), int64(speed), done, total)
		}
	case core.EventDone:
		if j, ok := data.(core.Job); ok {
			l.OnDownloadComplete(j.Filename, j.Path(), j.BatchID != "")
		}
	case core.EventSettings:
		if s, ok := data.(core.Settings); ok {
			l.OnMultiLinkChanged(s.MultiLink)
		}
	}
	if name == core.EventJob || name == core.EventProgress || name == core.EventRemoved {
		e.notifyActive()
	}
}

func randomToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
