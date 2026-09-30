package core

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Route is one way for a download to reach the server: a network link
// (Ethernet, Wi-Fi, a phone hotspot…) and/or a proxy. A download with several
// routes spreads its connections across all of them.
type Route struct {
	ID     string // stable key (the link ID, or "default")
	Label  string // e.g. "Wi-Fi (wlan0) · VLESS Germany"
	Kind   string // link kind, for icons: ethernet | wifi | cellular | usb | vpn | other
	Client *http.Client
}

// Router picks the routes for a download. proxy is the job's proxy choice
// ("" = the default from settings).
type Router interface {
	Routes(rawURL, proxy string) ([]Route, error)
}

// SetRouter installs the router (proxies + multi-link). Without one every
// download uses a single direct route.
func (m *Manager) SetRouter(r Router) {
	m.mu.Lock()
	m.router = r
	m.mu.Unlock()
}

func (m *Manager) routesFor(job *Job) ([]Route, error) {
	m.mu.Lock()
	r := m.router
	m.mu.Unlock()
	if r == nil {
		return []Route{{ID: "default", Client: m.client}}, nil
	}
	rs, err := r.Routes(job.URL, job.Proxy)
	if err != nil {
		return nil, err
	}
	if len(rs) == 0 {
		return []Route{{ID: "default", Client: m.client}}, nil
	}
	return rs, nil
}

// routeState tracks one route while a download runs.
type routeState struct {
	Route
	bytes atomic.Int64
	conns atomic.Int32
	dead  atomic.Bool

	// speed sampling (guarded by routeSet.mu)
	lastBytes int64
	lastAt    time.Time
	speed     float64
}

// routeSet is the runner's view of its routes.
type routeSet struct {
	mu   sync.Mutex
	list []*routeState
	next int
}

func newRouteSet(rs []Route) *routeSet {
	s := &routeSet{}
	for _, r := range rs {
		s.list = append(s.list, &routeState{Route: r, lastAt: time.Now()})
	}
	return s
}

func (s *routeSet) all() []*routeState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*routeState(nil), s.list...)
}

// alive returns routes that haven't been dropped.
func (s *routeSet) alive() []*routeState {
	var out []*routeState
	for _, r := range s.all() {
		if !r.dead.Load() {
			out = append(out, r)
		}
	}
	return out
}

// pick returns the next live route, round-robin.
func (s *routeSet) pick() *routeState {
	s.mu.Lock()
	defer s.mu.Unlock()
	for range s.list {
		r := s.list[s.next%len(s.list)]
		s.next++
		if !r.dead.Load() {
			return r
		}
	}
	return nil
}

// drop marks rt as unusable for this download if at least one other route is
// still alive. It returns false when rt is the last one (the caller then
// fails the download with the error, as before).
func (s *routeSet) drop(rt *routeState) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rt.dead.Load() {
		return true // another connection on this route already dropped it
	}
	for _, r := range s.list {
		if r != rt && !r.dead.Load() {
			rt.dead.Store(true)
			return true
		}
	}
	return false
}

func (s *routeSet) multi() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.list) > 1 || (len(s.list) == 1 && s.list[0].Label != "")
}

// stats samples per-route speed for progress events.
func (s *routeSet) stats(now time.Time) []PathStat {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]PathStat, 0, len(s.list))
	for _, r := range s.list {
		b := r.bytes.Load()
		if dt := now.Sub(r.lastAt).Seconds(); dt > 0 {
			inst := float64(b-r.lastBytes) / dt
			if r.speed == 0 {
				r.speed = inst
			} else {
				r.speed = 0.35*inst + 0.65*r.speed
			}
		}
		r.lastBytes, r.lastAt = b, now
		sp := r.speed
		if r.dead.Load() {
			sp = 0
		}
		out = append(out, PathStat{ID: r.ID, Label: r.Label, Kind: r.Kind, Speed: sp, Conns: int(r.conns.Load())})
	}
	return out
}

// probeAny probes through every route at once and returns the first success,
// so a dead link (e.g. one that can't reach this server) doesn't delay the
// start. Routes whose probe fails are dropped for this download.
func (r *runner) probeAny(job *Job) (ProbeResult, error) {
	all := r.routes.all()
	if len(all) == 1 {
		return Probe(r.ctx, all[0].Client, job.URL, job.Headers)
	}
	type result struct {
		rt  *routeState
		pr  ProbeResult
		err error
	}
	ch := make(chan result, len(all))
	for _, rt := range all {
		go func() {
			pr, err := Probe(r.ctx, rt.Client, job.URL, job.Headers)
			ch <- result{rt, pr, err}
		}()
	}
	var firstErr error
	for pending := len(all); pending > 0; pending-- {
		x := <-ch
		if x.err == nil {
			// The rest finish in the background; drop the ones that fail.
			go func(n int) {
				for ; n > 0; n-- {
					if y := <-ch; y.err != nil && r.ctx.Err() == nil {
						r.routes.drop(y.rt)
					}
				}
			}(pending - 1)
			return x.pr, nil
		}
		if r.ctx.Err() != nil {
			return ProbeResult{}, r.ctx.Err()
		}
		if firstErr == nil {
			firstErr = x.err
		}
		r.routes.drop(x.rt)
	}
	return ProbeResult{}, firstErr
}

// shouldDropRoute decides whether an error is specific to one route (so the
// download can continue on the others) rather than to the file itself.
func shouldDropRoute(err error) bool {
	return err != nil && !errors.Is(err, context.Canceled)
}
