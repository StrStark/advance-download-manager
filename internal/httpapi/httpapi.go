// Package httpapi exposes the service over HTTP: a JSON RPC endpoint that
// mirrors the desktop bindings, server-sent events for live updates, and the
// UI assets. Used by the headless server and the Android app.
package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/StrStark/advance-download-manager/internal/core"
	"github.com/StrStark/advance-download-manager/internal/service"
)

// ---------- HTTP ----------

// Config controls how the API is exposed.
type Config struct {
	// Kind is reported to the UI ("server" or "android").
	Kind string
	// Basic auth (browser prompt). Empty Pass disables it.
	User, Pass string
	// Token auth for embedded use (Android): the UI is opened with ?token=…,
	// which sets a cookie used for every later request. Empty disables it.
	Token string
	// When set, every download folder must live under DownloadRoot.
	DownloadRoot string
	// PendingURLs returns (once) links handed to the app before the UI loaded.
	PendingURLs func() []string
}

// Handler serves the UI assets and the API for svc. Events emitted through
// hub are streamed to connected browsers.
func Handler(cfg Config, svc *service.Service, hub *Hub, assets fs.FS) http.Handler {
	if cfg.Kind == "" {
		cfg.Kind = "server"
	}
	if cfg.DownloadRoot != "" {
		cfg.DownloadRoot = filepath.Clean(cfg.DownloadRoot)
	}
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(assets))
	mux.HandleFunc("GET /api/ping", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"kind": cfg.Kind})
	})
	mux.HandleFunc("GET /api/events", hub.serve)
	mux.HandleFunc("POST /api/call/{method}", func(w http.ResponseWriter, r *http.Request) {
		var args []json.RawMessage
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<20)).Decode(&args); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request body"})
			return
		}
		res, err := dispatch(cfg, svc, r.PathValue("method"), args)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"result": res})
	})
	// Lets the browser save a finished download to the viewer's computer.
	mux.HandleFunc("GET /api/file/{id}", func(w http.ResponseWriter, r *http.Request) {
		p, err := svc.CompletedPath(r.PathValue("id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", pathEscape(filepath.Base(p))))
		http.ServeFile(w, r, p)
	})
	return withAuth(cfg, mux)
}

const tokenCookie = "adm_token"

func withAuth(cfg Config, next http.Handler) http.Handler {
	if cfg.Pass == "" && cfg.Token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cfg.Token != "" {
			if q := r.URL.Query().Get("token"); q != "" && eq(q, cfg.Token) {
				http.SetCookie(w, &http.Cookie{Name: tokenCookie, Value: cfg.Token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
				next.ServeHTTP(w, r)
				return
			}
			if c, err := r.Cookie(tokenCookie); err == nil && eq(c.Value, cfg.Token) {
				next.ServeHTTP(w, r)
				return
			}
			if cfg.Pass == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		u, p, ok := r.BasicAuth()
		if !ok || !eq(u, cfg.User) || !eq(p, cfg.Pass) {
			w.Header().Set("WWW-Authenticate", `Basic realm="ADM", charset="UTF-8"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func eq(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func pathEscape(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte("-._~", c) >= 0 {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// ---------- RPC ----------

func arg[T any](args []json.RawMessage, i int) (T, error) {
	var v T
	if i >= len(args) {
		return v, fmt.Errorf("missing argument %d", i+1)
	}
	err := json.Unmarshal(args[i], &v)
	return v, err
}

// dispatch maps a method name to the service. The method set mirrors the
// desktop bindings so the frontend can use either transport.
func dispatch(cfg Config, s *service.Service, method string, a []json.RawMessage) (any, error) {
	switch method {
	case "Kind":
		return cfg.Kind, nil
	case "GetState":
		return s.GetState(), nil
	case "PendingURLs":
		if cfg.PendingURLs != nil {
			return cfg.PendingURLs(), nil
		}
		return []string{}, nil
	case "Probe":
		u, err := arg[string](a, 0)
		if err != nil {
			return nil, err
		}
		h, _ := arg[map[string]string](a, 1)
		via, _ := arg[string](a, 2)
		return s.Probe(u, h, via), nil
	case "ProbeMany":
		session, err := arg[string](a, 0)
		if err != nil {
			return nil, err
		}
		urls, err := arg[[]string](a, 1)
		if err != nil {
			return nil, err
		}
		via, _ := arg[string](a, 2)
		s.ProbeMany(session, urls, via)
		return nil, nil
	case "ExtractURLs":
		t, err := arg[string](a, 0)
		return s.ExtractURLs(t), err
	case "ExpandPattern":
		p, err := arg[string](a, 0)
		if err != nil {
			return nil, err
		}
		return s.ExpandPattern(p)
	case "ParseImport":
		name, err := arg[string](a, 0)
		if err != nil {
			return nil, err
		}
		content, err := arg[string](a, 1)
		if err != nil {
			return nil, err
		}
		return s.ParseImport(name, content)
	case "DiskFree":
		d, err := arg[string](a, 0)
		if err != nil {
			return nil, err
		}
		return s.DiskFree(d)
	case "ChooseDirectory":
		// No native dialog in a browser; the UI edits the path as text.
		return arg[string](a, 0)
	case "AddDownload":
		req, err := arg[core.AddRequest](a, 0)
		if err != nil {
			return nil, err
		}
		if req.Dir, err = confine(cfg, req.Dir); err != nil {
			return nil, err
		}
		return s.AddDownload(req)
	case "CreateBatch":
		req, err := arg[core.BatchRequest](a, 0)
		if err != nil {
			return nil, err
		}
		if req.Dir, err = confine(cfg, req.Dir); err != nil {
			return nil, err
		}
		for i := range req.Items {
			if req.Items[i].Dir, err = confine(cfg, req.Items[i].Dir); err != nil {
				return nil, err
			}
		}
		return s.CreateBatch(req)
	case "Pause", "Resume":
		ids, err := arg[[]string](a, 0)
		if err != nil {
			return nil, err
		}
		if method == "Pause" {
			s.Pause(ids)
		} else {
			s.Resume(ids)
		}
		return nil, nil
	case "Remove":
		ids, err := arg[[]string](a, 0)
		if err != nil {
			return nil, err
		}
		del, _ := arg[bool](a, 1)
		s.Remove(ids, del)
		return nil, nil
	case "RetryFailed":
		b, err := arg[string](a, 0)
		s.RetryFailed(b)
		return nil, err
	case "UpdateBatch":
		b, err := arg[core.Batch](a, 0)
		if err != nil {
			return nil, err
		}
		s.UpdateBatch(b)
		return nil, nil
	case "SetSpeedLimit":
		bps, err := arg[int64](a, 0)
		s.SetSpeedLimit(bps)
		return nil, err
	case "AppInfo":
		return s.AppInfo(), nil
	case "CheckUpdate":
		return s.CheckUpdate()
	case "InstallUpdate":
		return nil, s.InstallUpdate()
	case "ListLinks":
		return s.ListLinks(), nil
	case "CheckLinks":
		return s.CheckLinks(), nil
	case "XrayAvailable":
		return s.XrayAvailable(), nil
	case "ParseProxies":
		t, err := arg[string](a, 0)
		return s.ParseProxies(t), err
	case "FetchSubscription":
		u, err := arg[string](a, 0)
		if err != nil {
			return nil, err
		}
		via, _ := arg[string](a, 1)
		return s.FetchSubscription(u, via)
	case "TestProxy":
		p, err := arg[core.ProxyProfile](a, 0)
		if err != nil {
			return nil, err
		}
		return s.TestProxy(p), nil
	case "UpdateSettings":
		st, err := arg[core.Settings](a, 0)
		if err != nil {
			return nil, err
		}
		if st.DownloadDir, err = confine(cfg, st.DownloadDir); err != nil {
			return nil, err
		}
		st.ClipboardWatch = false // not available headless
		return s.UpdateSettings(st), nil
	}
	return nil, fmt.Errorf("unknown method %q", method)
}

// confine keeps download folders inside ADM_DOWNLOAD_ROOT (when set), so the
// web UI can't write to arbitrary places such as the state directory.
// Relative paths are resolved against the root. Empty stays empty (default).
func confine(cfg Config, dir string) (string, error) {
	if cfg.DownloadRoot == "" || dir == "" {
		return dir, nil
	}
	if strings.HasPrefix(dir, "~") {
		dir = strings.TrimLeft(strings.TrimPrefix(dir, "~"), "/")
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(cfg.DownloadRoot, dir)
	}
	dir = filepath.Clean(dir)
	rel, err := filepath.Rel(cfg.DownloadRoot, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("folder must be inside %s", cfg.DownloadRoot)
	}
	return dir, nil
}

// ---------- server-sent events ----------

// Hub fans events out to connected browsers over server-sent events.
type Hub struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
}

// Clients is the number of connected UIs.
func (h *Hub) Clients() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

func NewHub() *Hub { return &Hub{clients: map[chan []byte]struct{}{}} }

// Emit implements core.Emitter and the service emit func.
func (h *Hub) Emit(name string, data any) {
	b, err := json.Marshal(map[string]any{"name": name, "data": data})
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		select {
		case c <- b:
		default:
			// Too slow: drop the client. The browser reconnects and resyncs.
			delete(h.clients, c)
			close(c)
		}
	}
}

func (h *Hub) serve(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	c := make(chan []byte, 512)
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		if _, ok := h.clients[c]; ok {
			delete(h.clients, c)
			close(c)
		}
		h.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case b, ok := <-c:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
	}
}
