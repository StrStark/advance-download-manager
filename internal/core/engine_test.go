package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testServer struct {
	data         []byte
	noRanges     bool
	noLength     bool
	failEvery    int32 // every Nth ranged request fails with 503
	delayPerKB   time.Duration
	reqs, ranged atomic.Int32
}

func (ts *testServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	n := ts.reqs.Add(1)
	w.Header().Set("ETag", `"v1"`)
	w.Header().Set("Content-Type", "application/octet-stream")
	body := ts.data
	rng := r.Header.Get("Range")
	if rng != "" && !ts.noRanges {
		var a, b int64
		if _, err := fmt.Sscanf(rng, "bytes=%d-%d", &a, &b); err != nil || a > b || a >= int64(len(ts.data)) {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		if ts.failEvery > 0 && ts.ranged.Add(1)%ts.failEvery == 0 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		b = min(b, int64(len(ts.data))-1)
		body = ts.data[a : b+1]
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", a, b, len(ts.data)))
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		if !ts.noRanges {
			w.Header().Set("Accept-Ranges", "bytes")
		}
		if !ts.noLength {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		}
		w.WriteHeader(http.StatusOK)
	}
	_ = n
	if r.Method == http.MethodHead {
		return
	}
	for len(body) > 0 {
		k := min(len(body), 16<<10)
		if _, err := w.Write(body[:k]); err != nil {
			return
		}
		body = body[k:]
		if ts.delayPerKB > 0 {
			time.Sleep(ts.delayPerKB * time.Duration(k/1024))
		}
	}
}

type recEmitter struct {
	mu   sync.Mutex
	done []Job
}

func (e *recEmitter) Emit(ev string, data any) {
	if ev == EventDone {
		e.mu.Lock()
		e.done = append(e.done, data.(Job))
		e.mu.Unlock()
	}
}

func randomData(n int) []byte {
	b := make([]byte, n)
	r := rand.New(rand.NewPCG(1, 2))
	for i := range b {
		b[i] = byte(r.Uint32())
	}
	return b
}

func newTestManager(t *testing.T) (*Manager, string, context.CancelFunc) {
	t.Helper()
	dir := t.TempDir()
	m := NewManager(NewStore(filepath.Join(dir, "state.json")), &recEmitter{})
	s := m.Settings()
	s.DownloadDir = filepath.Join(dir, "dl")
	s.MaxRetries = 4
	m.UpdateSettings(s)
	ctx, cancel := context.WithCancel(context.Background())
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	return m, s.DownloadDir, cancel
}

func waitStatus(t *testing.T, m *Manager, id string, want Status, timeout time.Duration) Job {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, j := range m.Snapshot().Jobs {
			if j.ID == id && j.Status == want {
				return j
			}
			if j.ID == id && want != StatusFailed && j.Status == StatusFailed {
				t.Fatalf("job failed: %s", j.Error)
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	for _, j := range m.Snapshot().Jobs {
		if j.ID == id {
			t.Fatalf("timeout waiting for %s, status=%s err=%s", want, j.Status, j.Error)
		}
	}
	t.Fatalf("job %s not found", id)
	return Job{}
}

func checkFile(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("content mismatch: got %d bytes sha %x, want %d bytes sha %x",
			len(got), sha256.Sum256(got), len(want), sha256.Sum256(want))
	}
}

func TestSegmentedDownload(t *testing.T) {
	data := randomData(9<<20 + 12345)
	srv := httptest.NewServer(&testServer{data: data})
	defer srv.Close()
	m, dir, cancel := newTestManager(t)
	defer cancel()

	j, err := m.Add(AddRequest{URL: srv.URL + "/files/big.bin", Connections: 6})
	if err != nil {
		t.Fatal(err)
	}
	done := waitStatus(t, m, j.ID, StatusCompleted, 20*time.Second)
	if done.Filename != "big.bin" || !done.Resumable || done.Size != int64(len(data)) {
		t.Fatalf("unexpected job: %+v", done)
	}
	checkFile(t, filepath.Join(dir, "big.bin"), data)
	if _, err := os.Stat(filepath.Join(dir, "big.bin.part")); !os.IsNotExist(err) {
		t.Fatal(".part file left behind")
	}
}

func TestWorkStealingSplitsLargestSegment(t *testing.T) {
	r := newRunner(nil, "x")
	big := &Segment{Start: 0, End: 10<<20 - 1, Done: 2 << 20}
	small := &Segment{Start: 10 << 20, End: 11<<20 - 1}
	r.segs = []*Segment{big, small}
	r.owned[big] = true
	r.owned[small] = true

	tail := r.take(nil)
	if tail == nil {
		t.Fatal("expected a split")
	}
	if big.End+1 != tail.Start || tail.End != 10<<20-1 {
		t.Fatalf("bad split: big=%+v tail=%+v", *big, *tail)
	}
	if big.Remaining() != 4<<20 || tail.Remaining() != 4<<20 {
		t.Fatalf("uneven halves: %d / %d", big.Remaining(), tail.Remaining())
	}
	// Nothing big enough left to split below 2*minSplit.
	big.Done = big.End - big.Start
	tail.Done = tail.End - tail.Start
	small.Done = small.End - small.Start
	if s := r.take(nil); s != nil {
		t.Fatalf("unexpected split %+v", *s)
	}
}

func TestResumeWithFewerSegmentsThanConnections(t *testing.T) {
	// A slow download with 1 connection, paused, then resumed with 4:
	// stealing must fan the single remaining segment out.
	data := randomData(8 << 20)
	srv := httptest.NewServer(&testServer{data: data, delayPerKB: 250 * time.Microsecond})
	defer srv.Close()
	m, dir, cancel := newTestManager(t)
	defer cancel()

	j, _ := m.Add(AddRequest{URL: srv.URL + "/a.bin", Connections: 1})
	waitStatus(t, m, j.ID, StatusDownloading, 5*time.Second)
	time.Sleep(200 * time.Millisecond)
	m.Pause([]string{j.ID})
	waitStatus(t, m, j.ID, StatusPaused, 5*time.Second)
	m.mu.Lock()
	m.jobs[j.ID].Connections = 4
	m.mu.Unlock()
	m.Resume([]string{j.ID})
	done := waitStatus(t, m, j.ID, StatusCompleted, 30*time.Second)
	if len(done.Segments) < 4 {
		t.Fatalf("expected >= 4 segments after stealing, got %d", len(done.Segments))
	}
	checkFile(t, filepath.Join(dir, "a.bin"), data)
}

func TestPauseResume(t *testing.T) {
	data := randomData(8 << 20)
	srv := httptest.NewServer(&testServer{data: data, delayPerKB: 500 * time.Microsecond})
	defer srv.Close()
	m, dir, cancel := newTestManager(t)
	defer cancel()

	j, _ := m.Add(AddRequest{URL: srv.URL + "/p.bin", Connections: 4})
	waitStatus(t, m, j.ID, StatusDownloading, 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	m.Pause([]string{j.ID})
	paused := waitStatus(t, m, j.ID, StatusPaused, 5*time.Second)
	time.Sleep(100 * time.Millisecond)
	paused = waitStatus(t, m, j.ID, StatusPaused, time.Second)
	if paused.Downloaded <= 0 || paused.Downloaded >= int64(len(data)) {
		t.Fatalf("expected partial progress, got %d", paused.Downloaded)
	}
	m.Resume([]string{j.ID})
	waitStatus(t, m, j.ID, StatusCompleted, 30*time.Second)
	checkFile(t, filepath.Join(dir, "p.bin"), data)
}

func TestRetriesTransientErrors(t *testing.T) {
	data := randomData(4 << 20)
	srv := httptest.NewServer(&testServer{data: data, failEvery: 3})
	defer srv.Close()
	m, dir, cancel := newTestManager(t)
	defer cancel()

	j, _ := m.Add(AddRequest{URL: srv.URL + "/r.bin", Connections: 4})
	waitStatus(t, m, j.ID, StatusCompleted, 60*time.Second)
	checkFile(t, filepath.Join(dir, "r.bin"), data)
}

func TestNoRangeServerStreams(t *testing.T) {
	data := randomData(3<<20 + 7)
	srv := httptest.NewServer(&testServer{data: data, noRanges: true})
	defer srv.Close()
	m, dir, cancel := newTestManager(t)
	defer cancel()

	j, _ := m.Add(AddRequest{URL: srv.URL + "/n.bin"})
	done := waitStatus(t, m, j.ID, StatusCompleted, 20*time.Second)
	if done.Resumable {
		t.Fatal("expected non-resumable")
	}
	checkFile(t, filepath.Join(dir, "n.bin"), data)
}

// A server that advertises ranges but ignores them: the job must restart on a
// single connection under the same name, leaving no stray .part file.
func TestRangeIgnoredFallbackKeepsName(t *testing.T) {
	data := randomData(3 << 20)
	inner := &testServer{data: data, noRanges: true}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes") // lie
		inner.ServeHTTP(w, r)
	}))
	defer srv.Close()
	m, dir, cancel := newTestManager(t)
	defer cancel()

	j, _ := m.Add(AddRequest{URL: srv.URL + "/liar.bin", Connections: 4})
	done := waitStatus(t, m, j.ID, StatusCompleted, 20*time.Second)
	if done.Filename != "liar.bin" || !done.SingleConn {
		t.Fatalf("got filename=%q singleConn=%v", done.Filename, done.SingleConn)
	}
	checkFile(t, filepath.Join(dir, "liar.bin"), data)
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("expected only liar.bin, found %d entries", len(entries))
	}
}

func TestUnknownLength(t *testing.T) {
	data := randomData(1<<20 + 3)
	srv := httptest.NewServer(&testServer{data: data, noRanges: true, noLength: true})
	defer srv.Close()
	m, dir, cancel := newTestManager(t)
	defer cancel()

	j, _ := m.Add(AddRequest{URL: srv.URL + "/u.bin"})
	done := waitStatus(t, m, j.ID, StatusCompleted, 20*time.Second)
	if done.Size != int64(len(data)) {
		t.Fatalf("size %d, want %d", done.Size, len(data))
	}
	checkFile(t, filepath.Join(dir, "u.bin"), data)
}

func TestBatchSequentialAndNameConflict(t *testing.T) {
	data := randomData(2 << 20)
	srv := httptest.NewServer(&testServer{data: data})
	defer srv.Close()
	m, dir, cancel := newTestManager(t)
	defer cancel()

	var items []AddRequest
	for i := range 3 {
		items = append(items, AddRequest{URL: fmt.Sprintf("%s/ep%d.bin", srv.URL, i)})
	}
	items = append(items, AddRequest{URL: srv.URL + "/ep0.bin"}) // duplicate name
	b, err := m.AddBatch(BatchRequest{Name: "Season", Sequential: true, Items: items})
	if err != nil {
		t.Fatal(err)
	}
	ids := m.BatchJobIDs(b.ID)
	if len(ids) != 4 {
		t.Fatalf("got %d jobs", len(ids))
	}
	for _, id := range ids {
		waitStatus(t, m, id, StatusCompleted, 20*time.Second)
	}
	for _, name := range []string{"ep0.bin", "ep1.bin", "ep2.bin", "ep0 (1).bin"} {
		checkFile(t, filepath.Join(dir, name), data)
	}
}

func TestFailedAndRetry(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	data := randomData(1 << 20)
	inner := &testServer{data: data}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			http.Error(w, "nope", http.StatusForbidden)
			return
		}
		inner.ServeHTTP(w, r)
	}))
	defer srv.Close()
	m, dir, cancel := newTestManager(t)
	defer cancel()

	b, _ := m.AddBatch(BatchRequest{Items: []AddRequest{{URL: srv.URL + "/f.bin"}}})
	id := m.BatchJobIDs(b.ID)[0]
	failed := waitStatus(t, m, id, StatusFailed, 10*time.Second)
	if !strings.Contains(failed.Error, "403") {
		t.Fatalf("unexpected error %q", failed.Error)
	}
	fail.Store(false)
	m.RetryFailed(b.ID)
	waitStatus(t, m, id, StatusCompleted, 10*time.Second)
	checkFile(t, filepath.Join(dir, "f.bin"), data)
}

func TestPersistenceAcrossRestart(t *testing.T) {
	data := randomData(6 << 20)
	srv := httptest.NewServer(&testServer{data: data, delayPerKB: 400 * time.Microsecond})
	defer srv.Close()
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "state.json"))

	m1 := NewManager(store, nil)
	s := m1.Settings()
	s.DownloadDir = filepath.Join(dir, "dl")
	m1.UpdateSettings(s)
	ctx1, cancel1 := context.WithCancel(context.Background())
	m1.Start(ctx1)
	j, _ := m1.Add(AddRequest{URL: srv.URL + "/persist.bin", Connections: 4})
	waitStatus(t, m1, j.ID, StatusDownloading, 5*time.Second)
	time.Sleep(400 * time.Millisecond)
	m1.Shutdown()
	cancel1()

	m2 := NewManager(store, nil)
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	m2.Start(ctx2)
	waitStatus(t, m2, j.ID, StatusCompleted, 30*time.Second)
	checkFile(t, filepath.Join(dir, "dl", "persist.bin"), data)
}

func TestLimiter(t *testing.T) {
	l := NewLimiter(1 << 20) // 1 MiB/s
	start := time.Now()
	for range 8 {
		l.WaitN(context.Background(), 64<<10) // 512 KiB total
	}
	if el := time.Since(start); el < 250*time.Millisecond {
		t.Fatalf("limiter too permissive: %v", el)
	}
}

func TestSanitizeFilename(t *testing.T) {
	cases := map[string]string{
		"a/b\\c.txt":    "a_b_c.txt",
		`what?:"*".pdf`: "what_____.pdf",
		"con.txt":       "_con.txt",
		"trailing. . ":  "trailing",
		"..":            "download",
		"ok name.zip":   "ok name.zip",
	}
	for in, want := range cases {
		if got := SanitizeFilename(in); got != want {
			t.Errorf("SanitizeFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFilenameFromDisposition(t *testing.T) {
	got := FilenameFromResponse(`attachment; filename*=UTF-8''r%C3%A9sum%C3%A9.pdf`, nil, "")
	if got != "résumé.pdf" {
		t.Fatalf("got %q", got)
	}
}
