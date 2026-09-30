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
	"github.com/StrStark/advance-download-manager/internal/links"
	"github.com/StrStark/advance-download-manager/internal/netpath"
	"github.com/StrStark/advance-download-manager/internal/platform"
	"github.com/StrStark/advance-download-manager/internal/proxy"
)

// Event names emitted by the service itself (the engine's are in core).
const (
	EventProbeResult = "probe:result"
	EventClipboard   = "clipboard:url"
	EventExternal    = "external:urls"
)

// Service wraps a Manager with UI-facing operations.
type Service struct {
	M      *core.Manager
	router *netpath.Router
	ctx    context.Context
	emit   func(name string, data any)
}

func New(m *core.Manager) *Service {
	r := netpath.New(m.Settings)
	m.SetRouter(r)
	return &Service{M: m, router: r, ctx: context.Background(), emit: func(string, any) {}}
}

// Attach connects the service to a running UI and starts the engine.
func (s *Service) Attach(ctx context.Context, emit func(string, any)) error {
	s.ctx, s.emit = ctx, emit
	return s.M.Start(ctx)
}

// Emit sends an event to the attached UI.
func (s *Service) Emit(name string, data any) { s.emit(name, data) }

func (s *Service) GetState() core.State { return s.M.Snapshot() }

// Probe checks a URL through the given proxy choice ("" = default).
func (s *Service) Probe(rawURL string, headers map[string]string, proxyChoice string) core.ProbeResult {
	ctx, cancel := context.WithTimeout(s.ctx, 20*time.Second)
	defer cancel()
	rawURL = strings.TrimSpace(rawURL)
	c := s.M.Client()
	if rs, err := s.router.Routes(rawURL, proxyChoice); err == nil && len(rs) > 0 {
		c = rs[0].Client
	} else if err != nil {
		return core.ProbeResult{URL: rawURL, Size: -1, Error: err.Error()}
	}
	r, err := core.Probe(ctx, c, rawURL, headers)
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
func (s *Service) ProbeMany(session string, urls []string, proxyChoice string) {
	go func() {
		jobs := make(chan int)
		var wg sync.WaitGroup
		for range min(8, len(urls)) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range jobs {
					s.emit(EventProbeResult, probeEvent{Session: session, Index: i, Result: s.Probe(urls[i], nil, proxyChoice)})
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

func (s *Service) Pause(ids []string)                    { s.M.Pause(ids) }
func (s *Service) Resume(ids []string)                   { s.M.Resume(ids) }
func (s *Service) Remove(ids []string, deleteFiles bool) { s.M.Remove(ids, deleteFiles) }
func (s *Service) RetryFailed(batchID string)            { s.M.RetryFailed(batchID) }
func (s *Service) UpdateBatch(b core.Batch)              { s.M.UpdateBatch(b) }
func (s *Service) SetSpeedLimit(bps int64)               { s.M.SetSpeedLimit(bps) }
func (s *Service) UpdateSettings(st core.Settings) core.Settings {
	for i := range st.Proxies {
		if st.Proxies[i].ID == "" {
			st.Proxies[i].ID = core.NewID()
		}
	}
	for i := range st.Subscriptions {
		if st.Subscriptions[i].ID == "" {
			st.Subscriptions[i].ID = core.NewID()
		}
	}
	return s.M.UpdateSettings(st)
}

// ---------- network: links and proxies ----------

// LinkInfo is a network link plus whether multi-link downloads use it.
type LinkInfo struct {
	links.Link
	Enabled bool `json:"enabled"`
}

// ListLinks returns the device's network connections.
func (s *Service) ListLinks() []LinkInfo {
	st := s.M.Settings()
	var out []LinkInfo
	for _, l := range links.List() {
		out = append(out, LinkInfo{Link: l, Enabled: netpath.Enabled(st, l)})
	}
	return out
}

// LinkCheck is the result of a reachability test through one link.
type LinkCheck struct {
	LatencyMs int64  `json:"latencyMs"`
	Error     string `json:"error,omitempty"`
}

// CheckLinks tests every link in parallel by opening a connection through it.
func (s *Service) CheckLinks() map[string]LinkCheck {
	ls := links.List()
	out := make(map[string]LinkCheck, len(ls))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, l := range ls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
			defer cancel()
			d, err := links.Check(ctx, l, "1.1.1.1:443")
			if err != nil {
				d, err = links.Check(ctx, l, "cloudflare.com:443")
			}
			c := LinkCheck{LatencyMs: d.Milliseconds()}
			if err != nil {
				c = LinkCheck{Error: "no internet through this connection"}
			}
			mu.Lock()
			out[l.ID] = c
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out
}

// ProxyParse is the result of reading proxy links or a subscription.
type ProxyParse struct {
	Profiles []core.ProxyProfile `json:"profiles"`
	Errors   []string            `json:"errors"`
}

// ParseProxies reads pasted proxy URLs / share links (one per line, or a
// base64 block) into new profiles with IDs. Nothing is saved.
func (s *Service) ParseProxies(text string) ProxyParse {
	ps, errs := proxy.ParseMany(text)
	for i := range ps {
		ps[i].ID = core.NewID()
	}
	return ProxyParse{Profiles: ps, Errors: errs}
}

// FetchSubscription downloads a subscription URL (through the given proxy
// choice, "" = direct) and returns its servers. Nothing is saved.
func (s *Service) FetchSubscription(subURL, via string) (ProxyParse, error) {
	if via == "" {
		via = "direct"
	}
	c, err := s.router.Client(via)
	if err != nil {
		return ProxyParse{}, err
	}
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()
	ps, errs, err := proxy.FetchSubscription(ctx, c, strings.TrimSpace(subURL))
	if err != nil {
		return ProxyParse{Errors: errs}, err
	}
	for i := range ps {
		ps[i].ID = core.NewID()
	}
	return ProxyParse{Profiles: ps, Errors: errs}, nil
}

// TestProxy checks a proxy by fetching a small page through it.
func (s *Service) TestProxy(p core.ProxyProfile) proxy.TestResult {
	return proxy.Test(s.ctx, &p, nil)
}

// XrayAvailable tells the UI whether V2Ray/Xray protocols are built in.
func (s *Service) XrayAvailable() bool { return proxy.XrayAvailable }

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
