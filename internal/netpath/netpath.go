// Package netpath decides how each download reaches the internet: which
// network links to use (multi-link aggregation) and which proxy, based on
// the user's settings. It implements core.Router.
package netpath

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"

	"github.com/StrStark/advance-download-manager/internal/core"
	"github.com/StrStark/advance-download-manager/internal/links"
	"github.com/StrStark/advance-download-manager/internal/proxy"
)

// Router builds and caches one HTTP client per (proxy, link) pair.
type Router struct {
	settings func() core.Settings

	mu    sync.Mutex
	cache map[string]*entry
}

type entry struct {
	client *http.Client
	closer io.Closer
}

func New(settings func() core.Settings) *Router {
	return &Router{settings: settings, cache: map[string]*entry{}}
}

// Routes implements core.Router.
func (r *Router) Routes(rawURL, choice string) ([]core.Route, error) {
	s := r.settings()
	prof, err := Resolve(s, choice)
	if err != nil {
		return nil, err
	}
	if prof != nil && prof.Type != "system" {
		if u, err := url.Parse(rawURL); err == nil && proxy.Bypass(u.Hostname(), s.ProxyBypass) {
			prof = nil
		}
	}

	var ls []*links.Link
	for _, l := range ActiveLinks(s) {
		l := l
		ls = append(ls, &l)
	}
	if len(ls) < 2 {
		ls = []*links.Link{nil} // one link: let the OS route as usual
	}

	var out []core.Route
	for _, l := range ls {
		c, err := r.client(prof, l)
		if err != nil {
			return nil, err
		}
		rt := core.Route{ID: "default", Client: c}
		if l != nil {
			rt.ID, rt.Label, rt.Kind = l.ID, l.Label, string(l.Kind)
		}
		if prof != nil {
			if rt.Label != "" {
				rt.Label += " · " + prof.Name
			} else {
				rt.Label = prof.Name
			}
		}
		out = append(out, rt)
	}
	return out, nil
}

// Client returns a single client for the given proxy choice over the default
// route (used for subscriptions and tests).
func (r *Router) Client(choice string) (*http.Client, error) {
	prof, err := Resolve(r.settings(), choice)
	if err != nil {
		return nil, err
	}
	return r.client(prof, nil)
}

// Close shuts down every cached transport (and its Xray instance).
func (r *Router) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, e := range r.cache {
		e.closer.Close()
		delete(r.cache, k)
	}
}

func (r *Router) client(p *core.ProxyProfile, l *links.Link) (*http.Client, error) {
	key := "direct"
	if p != nil {
		key = p.Type + "|" + p.URL
	}
	if l != nil {
		key += fmt.Sprintf("|%s|%s|%d", l.ID, l.Name, l.Handle)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.cache[key]; ok {
		return e.client, nil
	}
	t, closer, err := proxy.NewTransport(p, l)
	if err != nil {
		name := "proxy"
		if p != nil {
			name = p.Name
		}
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	e := &entry{client: &http.Client{Transport: t}, closer: closer}
	r.cache[key] = e
	return e.client, nil
}

// Resolve maps a proxy choice to a profile: "" uses the default setting,
// "direct" means none, "system" the environment's proxy, anything else a
// profile ID.
func Resolve(s core.Settings, choice string) (*core.ProxyProfile, error) {
	if choice == "" {
		choice = s.DefaultProxy
	}
	switch choice {
	case "", "direct":
		return nil, nil
	case "system":
		return proxy.System, nil
	}
	for i := range s.Proxies {
		if s.Proxies[i].ID == choice {
			p := s.Proxies[i]
			return &p, nil
		}
	}
	return nil, fmt.Errorf("proxy %q no longer exists; pick another in the Network panel", choice)
}

// ActiveLinks returns the links a download should use when multi-link is on:
// the user's selection, or every connected non-VPN link. It returns nothing
// when multi-link is off.
func ActiveLinks(s core.Settings) []links.Link {
	if !s.MultiLink {
		return nil
	}
	var out []links.Link
	for _, l := range links.List() {
		if Enabled(s, l) {
			out = append(out, l)
		}
	}
	return out
}

// Enabled reports whether l takes part in multi-link downloads.
func Enabled(s core.Settings, l links.Link) bool {
	if len(s.Links) == 0 {
		return l.Kind != links.VPN
	}
	for _, id := range s.Links {
		if id == l.ID {
			return true
		}
	}
	return false
}
