//go:build server

// Headless mode: `go build -tags server` produces a pure-Go binary (no
// GTK/WebKit) that runs the engine and serves the same UI over HTTP. This is
// what the Docker image runs.
package main

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/StrStark/advance-download-manager/internal/core"
	"github.com/StrStark/advance-download-manager/internal/httpapi"
	"github.com/StrStark/advance-download-manager/internal/platform"
	"github.com/StrStark/advance-download-manager/internal/service"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}
	listen := envOr("ADM_LISTEN", ":8080")
	cfg := httpapi.Config{
		Kind:         "server",
		User:         envOr("ADM_USER", "admin"),
		Pass:         os.Getenv("ADM_PASSWORD"),
		DownloadRoot: os.Getenv("ADM_DOWNLOAD_ROOT"),
	}
	if cfg.DownloadRoot != "" && os.Getenv("ADM_DOWNLOAD_DIR") == "" {
		os.Setenv("ADM_DOWNLOAD_DIR", cfg.DownloadRoot)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	hub := httpapi.NewHub()
	m := core.NewManager(core.NewStore(filepath.Join(platform.DataDir(), "state.json")), hub)
	svc := service.New(m)
	if err := svc.Attach(ctx, hub.Emit); err != nil {
		log.Fatalf("load state: %v", err)
	}

	dist, _ := fs.Sub(assets, "frontend/dist")
	srv := &http.Server{Addr: listen, Handler: httpapi.Handler(cfg, svc, hub, dist), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	if cfg.Pass == "" {
		log.Printf("WARNING: ADM_PASSWORD is not set; the web UI is open to anyone who can reach %s", listen)
	}
	log.Printf("ADM server listening on %s (data: %s, downloads: %s)", listen, platform.DataDir(), m.Settings().DownloadDir)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	m.Shutdown()
	log.Print("stopped")
}

// healthcheck pings the local server; used by the container HEALTHCHECK
// (the image has no shell or curl).
func healthcheck() int {
	addr := envOr("ADM_LISTEN", ":8080")
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	req, _ := http.NewRequest(http.MethodGet, "http://"+addr+"/api/ping", nil)
	if p := os.Getenv("ADM_PASSWORD"); p != "" {
		req.SetBasicAuth(envOr("ADM_USER", "admin"), p)
	}
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return 1
	}
	resp.Body.Close()
	return 0
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
