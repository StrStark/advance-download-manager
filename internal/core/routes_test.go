package core

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// fakeRouter hands every download the same fixed routes.
type fakeRouter struct{ routes []Route }

func (f fakeRouter) Routes(string, string) ([]Route, error) { return f.routes, nil }

// slowTransport throttles response bodies, simulating a slower link.
type slowTransport struct {
	delayPerChunk time.Duration
	requests      atomic.Int32
}

func (s *slowTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s.requests.Add(1)
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err == nil && s.delayPerChunk > 0 {
		resp.Body = &slowBody{ReadCloser: resp.Body, d: s.delayPerChunk}
	}
	return resp, err
}

type slowBody struct {
	io.ReadCloser
	d time.Duration
}

func (b *slowBody) Read(p []byte) (int, error) {
	time.Sleep(b.d)
	if len(p) > 16<<10 {
		p = p[:16<<10]
	}
	return b.ReadCloser.Read(p)
}

// forbidden rejects every request, like a CDN that locks links to one IP.
type forbidden struct{ calls atomic.Int32 }

func (f *forbidden) RoundTrip(req *http.Request) (*http.Response, error) {
	f.calls.Add(1)
	return &http.Response{StatusCode: 403, Status: "403 Forbidden", Body: http.NoBody, Header: http.Header{}, Request: req}, nil
}

func TestTwoRoutesShareTheWork(t *testing.T) {
	data := randomData(16 << 20)
	srv := httptest.NewServer(&testServer{data: data})
	defer srv.Close()
	m, dir, cancel := newTestManager(t)
	defer cancel()

	// ~5 MB/s vs ~1 MB/s per connection.
	fast := &slowTransport{delayPerChunk: 3 * time.Millisecond}
	slow := &slowTransport{delayPerChunk: 15 * time.Millisecond}
	m.SetRouter(fakeRouter{routes: []Route{
		{ID: "eth0", Label: "Ethernet (eth0)", Kind: "ethernet", Client: &http.Client{Transport: fast}},
		{ID: "wlan0", Label: "Wi-Fi (wlan0)", Kind: "wifi", Client: &http.Client{Transport: slow}},
	}})
	var sawPaths atomic.Bool
	m.SetEmitter(emitFunc(func(ev string, data any) {
		if ev == EventProgress {
			for _, p := range data.([]Progress) {
				if len(p.Paths) == 2 {
					sawPaths.Store(true)
				}
			}
		}
	}))

	j, _ := m.Add(AddRequest{URL: srv.URL + "/two.bin", Connections: 3})
	done := waitStatus(t, m, j.ID, StatusCompleted, 30*time.Second)
	checkFile(t, filepath.Join(dir, "two.bin"), data)
	if fast.requests.Load() < 2 || slow.requests.Load() < 2 {
		t.Fatalf("both routes should carry range requests: fast=%d slow=%d", fast.requests.Load(), slow.requests.Load())
	}
	paths := map[string]int{}
	for _, s := range done.Segments {
		paths[s.Path]++
	}
	if paths["eth0"] == 0 || paths["wlan0"] == 0 {
		t.Fatalf("segments should be tagged with both routes: %v", paths)
	}
	// Work stealing moves work to the faster link: it ends up with more bytes.
	var fastBytes, slowBytes int64
	for _, s := range done.Segments {
		if s.Path == "eth0" {
			fastBytes += s.Done
		} else {
			slowBytes += s.Done
		}
	}
	if fastBytes <= slowBytes {
		t.Fatalf("fast link should carry more: eth0=%d wlan0=%d", fastBytes, slowBytes)
	}
	t.Logf("bytes per route: eth0=%d MiB wlan0=%d MiB, segments %v", fastBytes>>20, slowBytes>>20, paths)
	if !sawPaths.Load() {
		t.Fatal("progress events should include per-route stats")
	}
}

func TestBrokenRouteIsDroppedNotFatal(t *testing.T) {
	data := randomData(6 << 20)
	srv := httptest.NewServer(&testServer{data: data})
	defer srv.Close()
	m, dir, cancel := newTestManager(t)
	defer cancel()

	bad := &forbidden{}
	m.SetRouter(fakeRouter{routes: []Route{
		{ID: "eth0", Label: "Ethernet", Client: &http.Client{}},
		{ID: "hotspot", Label: "Phone hotspot", Client: &http.Client{Transport: bad}},
	}})
	j, _ := m.Add(AddRequest{URL: srv.URL + "/locked.bin", Connections: 4})
	done := waitStatus(t, m, j.ID, StatusCompleted, 30*time.Second)
	checkFile(t, filepath.Join(dir, "locked.bin"), data)
	if bad.calls.Load() == 0 {
		t.Fatal("the broken route was never tried")
	}
	for _, s := range done.Segments {
		if s.Path == "hotspot" && s.Done > 0 {
			t.Fatalf("no bytes should come from the broken route: %+v", s)
		}
	}
}

func TestProbeFallsBackToWorkingRoute(t *testing.T) {
	data := randomData(2 << 20)
	srv := httptest.NewServer(&testServer{data: data})
	defer srv.Close()
	m, dir, cancel := newTestManager(t)
	defer cancel()
	m.SetRouter(fakeRouter{routes: []Route{
		{ID: "dead", Label: "Dead link", Client: &http.Client{Transport: &forbidden{}}},
		{ID: "ok", Label: "Working link", Client: &http.Client{}},
	}})
	j, _ := m.Add(AddRequest{URL: srv.URL + "/p.bin"})
	waitStatus(t, m, j.ID, StatusCompleted, 20*time.Second)
	checkFile(t, filepath.Join(dir, "p.bin"), data)
}

type emitFunc func(string, any)

func (f emitFunc) Emit(ev string, data any) { f(ev, data) }

var _ = fmt.Sprint
