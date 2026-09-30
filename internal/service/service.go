// Package service is the API surface shared by the desktop app (Wails
// bindings) and the headless server (HTTP). Both front-ends are thin
// adapters over these methods, so the UI sees the same behavior everywhere.
package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/StrStark/advance-download-manager/internal/batch"
	"github.com/StrStark/advance-download-manager/internal/core"
	"github.com/StrStark/advance-download-manager/internal/platform"
)

// Event names emitted by the service itself (the engine's are in core).
const (
	EventProbeResult = "probe:result"
	EventClipboard   = "clipboard:url"
	EventExternal    = "external:urls"
)

// Service wraps a Manager with UI-facing operations.
type Service struct {
	M    *core.Manager
	ctx  context.Context
	emit func(name string, data any)
}

func New(m *core.Manager) *Service {
	return &Service{M: m, ctx: context.Background(), emit: func(string, any) {}}
}

// Attach connects the service to a running UI and starts the engine.
func (s *Service) Attach(ctx context.Context, emit func(string, any)) error {
	s.ctx, s.emit = ctx, emit
	return s.M.Start(ctx)
}

// Emit sends an event to the attached UI.
func (s *Service) Emit(name string, data any) { s.emit(name, data) }

func (s *Service) GetState() core.State { return s.M.Snapshot() }

func (s *Service) Probe(rawURL string, headers map[string]string) core.ProbeResult {
	ctx, cancel := context.WithTimeout(s.ctx, 20*time.Second)
	defer cancel()
	r, err := core.Probe(ctx, s.M.Client(), strings.TrimSpace(rawURL), headers)
	if err != nil {
		r.Error = err.Error()
	}
	return r
}

type probeEvent struct {
	Session string           `json:"session"`
	Index   int              `json:"index"`
	Result  core.ProbeResult `json:"result"`
}

// ProbeMany checks URLs with a small worker pool and streams each result as
// a "probe:result" event.
func (s *Service) ProbeMany(session string, urls []string) {
	go func() {
		jobs := make(chan int)
		var wg sync.WaitGroup
		for range min(8, len(urls)) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range jobs {
					s.emit(EventProbeResult, probeEvent{Session: session, Index: i, Result: s.Probe(urls[i], nil)})
				}
			}()
		}
		for i := range urls {
			jobs <- i
		}
		close(jobs)
		wg.Wait()
	}()
}

func (s *Service) ExtractURLs(text string) []string { return batch.ExtractURLs(text) }

func (s *Service) ExpandPattern(p string) ([]string, error) {
	return batch.ExpandPattern(strings.TrimSpace(p))
}

// ParseImport parses an uploaded link list (used when there is no native
// file dialog, e.g. in the browser UI of the server).
func (s *Service) ParseImport(name, content string) ([]batch.Item, error) {
	return batch.ParseFile(name, strings.NewReader(content))
}

func (s *Service) DiskFree(dir string) (uint64, error) { return platform.DiskFree(dir) }

func (s *Service) AddDownload(req core.AddRequest) (core.Job, error) { return s.M.Add(req) }

func (s *Service) CreateBatch(req core.BatchRequest) (core.Batch, error) { return s.M.AddBatch(req) }

func (s *Service) Pause(ids []string)                            { s.M.Pause(ids) }
func (s *Service) Resume(ids []string)                           { s.M.Resume(ids) }
func (s *Service) Remove(ids []string, deleteFiles bool)         { s.M.Remove(ids, deleteFiles) }
func (s *Service) RetryFailed(batchID string)                    { s.M.RetryFailed(batchID) }
func (s *Service) UpdateBatch(b core.Batch)                      { s.M.UpdateBatch(b) }
func (s *Service) SetSpeedLimit(bps int64)                       { s.M.SetSpeedLimit(bps) }
func (s *Service) UpdateSettings(st core.Settings) core.Settings { return s.M.UpdateSettings(st) }

// CompletedPath returns the on-disk path of a finished download.
func (s *Service) CompletedPath(id string) (string, error) {
	j, ok := s.M.Job(id)
	if !ok {
		return "", errors.New("download not found")
	}
	if j.Status != core.StatusCompleted {
		return "", errors.New("download is not finished")
	}
	return j.Path(), nil
}

// OpenFile opens a finished download with the default application.
func (s *Service) OpenFile(id string) error {
	p, err := s.CompletedPath(id)
	if err != nil {
		return err
	}
	return platform.Open(p)
}

// ShowInFolder reveals a download in the system file manager.
func (s *Service) ShowInFolder(id string) error {
	j, ok := s.M.Job(id)
	if !ok {
		return errors.New("download not found")
	}
	if j.Status == core.StatusCompleted {
		return platform.Reveal(j.Path())
	}
	return platform.Reveal(j.PartPath())
}

// URLArgs filters command-line arguments down to http(s) URLs.
func URLArgs(args []string) []string {
	var out []string
	for _, a := range args {
		if strings.HasPrefix(a, "http://") || strings.HasPrefix(a, "https://") {
			out = append(out, a)
		}
	}
	return out
}
