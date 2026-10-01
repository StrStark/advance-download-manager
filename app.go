//go:build !server

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/StrStark/advance-download-manager/internal/autostart"
	"github.com/StrStark/advance-download-manager/internal/batch"
	"github.com/StrStark/advance-download-manager/internal/browser"
	"github.com/StrStark/advance-download-manager/internal/core"
	"github.com/StrStark/advance-download-manager/internal/platform"
	"github.com/StrStark/advance-download-manager/internal/service"
	"github.com/StrStark/advance-download-manager/internal/update"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is bound to the frontend: its exported methods (including those
// promoted from service.Service) are callable as window.go.main.App.<Method>.
type App struct {
	*service.Service
	ctx context.Context

	mu        sync.Mutex
	pending   []string           // URLs passed on the command line before the UI was ready
	pendingDL []browser.Download // downloads from the browser before the UI was ready
}

func NewApp(m *core.Manager, args []string) *App {
	a := &App{Service: service.New(m), pending: service.URLArgs(args), pendingDL: browser.DecodeArgs(args)}
	a.SetShell("desktop", filepath.Join(platform.DataDir(), "updates"))
	a.Installer = a.installUpdate
	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.M.SetEmitter(engineEvents{a})
	if err := a.Attach(ctx, func(name string, data any) { runtime.EventsEmit(ctx, name, data) }); err != nil {
		runtime.LogErrorf(ctx, "failed to load state: %v", err)
	}
	go a.watchClipboard(ctx)
	// Let browser extensions reach this copy of ADM (keeps the path current
	// after moves and updates).
	go func() {
		if err := browser.Register(); err != nil {
			runtime.LogWarningf(ctx, "browser integration: %v", err)
		}
	}()
	// Keep the login item in sync with the setting (e.g. after an update moved the binary).
	if s := a.M.Settings(); s.Autostart {
		_ = autostart.Set(true)
	}
}

// UpdateSettings saves settings and applies the desktop-only ones.
func (a *App) UpdateSettings(s core.Settings) core.Settings {
	out := a.Service.UpdateSettings(s)
	if out.Autostart != autostart.Enabled() {
		if err := autostart.Set(out.Autostart); err != nil {
			runtime.LogWarningf(a.ctx, "autostart: %v", err)
		}
	}
	return out
}

// installUpdate starts the platform installer and quits so files can be replaced.
func (a *App) installUpdate(path string) error {
	var keep []string
	for _, arg := range os.Args[1:] {
		if arg == autostart.BackgroundFlag {
			keep = append(keep, arg)
		}
	}
	if err := update.Apply(path, update.Detect("desktop"), keep); err != nil {
		return err
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		runtime.Quit(a.ctx)
	}()
	return nil
}

// PendingDownloads returns (once) browser downloads handed over at launch.
func (a *App) PendingDownloads() []browser.Download {
	a.mu.Lock()
	defer a.mu.Unlock()
	p := a.pendingDL
	a.pendingDL = nil
	return p
}

// raise brings the window to the front (it may be minimised or behind the browser).
func (a *App) raise() {
	runtime.WindowUnminimise(a.ctx)
	runtime.Show(a.ctx)
	runtime.WindowSetAlwaysOnTop(a.ctx, true)
	go func() {
		time.Sleep(600 * time.Millisecond)
		runtime.WindowSetAlwaysOnTop(a.ctx, false)
	}()
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
// It also receives downloads caught by the browser extension.
func (a *App) onSecondInstance(data options.SecondInstanceData) {
	a.raise()
	for _, d := range browser.DecodeArgs(data.Args) {
		runtime.EventsEmit(a.ctx, service.EventDownload, d)
	}
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
