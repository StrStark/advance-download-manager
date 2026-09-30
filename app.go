//go:build !server

package main

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/StrStark/advance-download-manager/internal/batch"
	"github.com/StrStark/advance-download-manager/internal/core"
	"github.com/StrStark/advance-download-manager/internal/platform"
	"github.com/StrStark/advance-download-manager/internal/service"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is bound to the frontend: its exported methods (including those
// promoted from service.Service) are callable as window.go.main.App.<Method>.
type App struct {
	*service.Service
	ctx context.Context

	mu      sync.Mutex
	pending []string // URLs passed on the command line before the UI was ready
}

func NewApp(m *core.Manager, args []string) *App {
	return &App{Service: service.New(m), pending: service.URLArgs(args)}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.M.SetEmitter(engineEvents{a})
	if err := a.Attach(ctx, func(name string, data any) { runtime.EventsEmit(ctx, name, data) }); err != nil {
		runtime.LogErrorf(ctx, "failed to load state: %v", err)
	}
	go a.watchClipboard(ctx)
}

func (a *App) shutdown(context.Context) { a.M.Shutdown() }

// engineEvents forwards engine events to the webview and adds desktop
// notifications for finished downloads.
type engineEvents struct{ a *App }

func (e engineEvents) Emit(name string, data any) {
	runtime.EventsEmit(e.a.ctx, name, data)
	if name == core.EventDone {
		if j, ok := data.(core.Job); ok && j.BatchID == "" && e.a.M.Settings().NotifyOnComplete {
			platform.Notify("Download complete", j.Filename)
		}
	}
}

// onSecondInstance receives URLs when `adm <url>` is run while ADM is open.
func (a *App) onSecondInstance(data options.SecondInstanceData) {
	runtime.WindowUnminimise(a.ctx)
	runtime.Show(a.ctx)
	if urls := service.URLArgs(data.Args); len(urls) > 0 {
		runtime.EventsEmit(a.ctx, service.EventExternal, urls)
	}
}

// Kind tells the UI which shell it runs in.
func (a *App) Kind() string { return "desktop" }

// PendingURLs returns (once) URLs given on the command line at launch.
func (a *App) PendingURLs() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	p := a.pending
	a.pending = nil
	return p
}

func (a *App) ImportFile() ([]batch.Item, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Import links",
		Filters: []runtime.FileFilter{
			{DisplayName: "Link lists (*.txt, *.csv, *.json)", Pattern: "*.txt;*.csv;*.json;*.list"},
			{DisplayName: "All files", Pattern: "*"},
		},
	})
	if err != nil || path == "" {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return batch.ParseFile(path, f)
}

func (a *App) ChooseDirectory(current string) (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:                "Choose download folder",
		DefaultDirectory:     current,
		CanCreateDirectories: true,
	})
}

// watchClipboard offers new links copied to the clipboard (opt-in).
func (a *App) watchClipboard(ctx context.Context) {
	last, _ := runtime.ClipboardGetText(ctx) // ignore whatever was there at launch
	t := time.NewTicker(1200 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if !a.M.Settings().ClipboardWatch {
			continue
		}
		text, err := runtime.ClipboardGetText(ctx)
		if err != nil || text == last {
			continue
		}
		last = text
		text = strings.TrimSpace(text)
		if strings.ContainsAny(text, " \n\t") || (!strings.HasPrefix(text, "http://") && !strings.HasPrefix(text, "https://")) {
			continue
		}
		runtime.EventsEmit(ctx, service.EventClipboard, text)
	}
}
