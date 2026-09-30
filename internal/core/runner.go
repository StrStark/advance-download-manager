package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// A segment is only split when both halves would be at least this big.
	minSplit  = 1 << 20
	chunkSize = 64 << 10
)

var errRangeIgnored = errors.New("server ignored range request")

// runner drives one job from probe to completion. The manager owns the Job
// struct; the runner owns the live segment list while it runs and publishes
// it back through Manager.syncRunner.
type runner struct {
	m      *Manager
	id     string
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	mu    sync.Mutex
	segs  []*Segment
	owned map[*Segment]bool

	downloaded atomic.Int64
	conns      atomic.Int32
	firstErr   error
	errOnce    sync.Once

	routes    *routeSet
	routesPtr atomic.Pointer[routeSet] // for the progress ticker
	wg        sync.WaitGroup
}

func newRunner(m *Manager, id string) *runner {
	ctx, cancel := context.WithCancel(context.Background())
	return &runner{m: m, id: id, ctx: ctx, cancel: cancel, done: make(chan struct{}), owned: map[*Segment]bool{}}
}

// segments returns a copy of the live segment list.
func (r *runner) segments() []Segment {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Segment, len(r.segs))
	for i, s := range r.segs {
		out[i] = *s
	}
	return out
}

func (r *runner) run() {
	defer close(r.done)
	err := r.download()
	r.m.finishRunner(r, err)
}

func (r *runner) download() error {
	job := r.m.jobCopy(r.id)
	if job == nil {
		return errors.New("job vanished")
	}

	// A job that ran before owns its name and .part file; only a brand-new job
	// needs to dodge existing files.
	ranBefore := job.Probed

	routes, err := r.m.routesFor(job)
	if err != nil {
		return err
	}
	r.routes = newRouteSet(routes)
	r.routesPtr.Store(r.routes)

	// Probe (always on start: also validates that a resume is still safe).
	pr, err := r.probeAny(job)
	if err != nil {
		return err
	}
	fresh := len(job.Segments) == 0 && job.Downloaded == 0
	if !fresh && (pr.Size != job.Size || !pr.Resumable || (job.ETag != "" && pr.ETag != "" && pr.ETag != job.ETag)) {
		// The remote file changed or can no longer be resumed: start over.
		job.Segments = nil
		job.Downloaded = 0
		fresh = true
	}
	if fresh && !job.Probed {
		// Keep a user-chosen filename; otherwise use what the server says.
		if job.Filename == "" {
			job.Filename = pr.Filename
		}
		job.Category = CategoryFor(job.Filename)
	}
	job.FinalURL, job.Size, job.Resumable = pr.FinalURL, pr.Size, pr.Resumable
	job.ETag, job.LastModified, job.ContentType = pr.ETag, pr.LastModified, pr.ContentType
	job.Probed = true

	if err := os.MkdirAll(job.Dir, 0o755); err != nil {
		return err
	}
	if fresh && !ranBefore {
		job.Filename = uniqueName(job.Dir, job.Filename)
	}
	r.m.applyProbe(r.id, job, fresh)

	flags := os.O_RDWR | os.O_CREATE
	if fresh {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(job.PartPath(), flags, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	if job.Resumable && job.Size > 0 && !job.SingleConn {
		err = r.segmented(f, job, fresh)
	} else {
		err = r.stream(f, job)
	}
	if err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	return nil
}

// segmented downloads with multiple connections and work stealing.
func (r *runner) segmented(f *os.File, job *Job, fresh bool) error {
	if fresh {
		if err := f.Truncate(job.Size); err != nil {
			return err
		}
		job.Segments = splitInitial(job.Size, job.Connections)
	}
	r.mu.Lock()
	r.segs = r.segs[:0]
	var done int64
	for i := range job.Segments {
		s := job.Segments[i]
		r.segs = append(r.segs, &s)
		done += s.Done
	}
	r.mu.Unlock()
	r.downloaded.Store(done)

	// Each route (network link / proxy) gets the job's connection count.
	workers := min(max(job.Connections, 1)*len(r.routes.alive()), 32)
	for range workers {
		rt := r.routes.pick()
		if rt == nil {
			break
		}
		seg := r.take(rt)
		if seg == nil {
			break
		}
		r.startWorker(f, job, seg, rt)
	}
	r.wg.Wait()
	if r.firstErr != nil {
		return r.firstErr
	}
	if err := r.ctx.Err(); err != nil {
		return err
	}
	for _, s := range r.segments() {
		if s.Remaining() > 0 {
			return errors.New("download incomplete")
		}
	}
	return nil
}

func splitInitial(size int64, conns int) []Segment {
	n := int64(max(conns, 1))
	if limit := size / minSplit; n > limit {
		n = limit
	}
	if n < 1 {
		n = 1
	}
	part := size / n
	segs := make([]Segment, 0, n)
	for i := int64(0); i < n; i++ {
		start := i * part
		end := start + part - 1
		if i == n-1 {
			end = size - 1
		}
		segs = append(segs, Segment{Start: start, End: end})
	}
	return segs
}

// startWorker runs one connection: it downloads seg, then keeps taking
// work until none is left. If its route fails while other routes are alive,
// the route is dropped for this download and its work moves elsewhere.
func (r *runner) startWorker(f *os.File, job *Job, seg *Segment, rt *routeState) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.conns.Add(1)
		rt.conns.Add(1)
		defer func() {
			r.conns.Add(-1)
			rt.conns.Add(-1)
		}()
		for seg != nil {
			if err := r.fetchRange(f, job, seg, rt); err != nil {
				if r.ctx.Err() == nil && shouldDropRoute(err) && r.routes.drop(rt) {
					r.release(seg)
					// Keep the connection count up on the remaining routes.
					if next := r.routes.pick(); next != nil {
						if s := r.take(next); s != nil {
							r.startWorker(f, job, s, next)
						}
					}
					return
				}
				r.fail(err)
				return
			}
			r.release(seg)
			seg = r.take(rt)
		}
	}()
}

// take returns an unowned incomplete segment, or splits the largest active
// one in half and returns the new tail. nil means there is nothing left.
func (r *runner) take(rt *routeState) *Segment {
	r.mu.Lock()
	defer r.mu.Unlock()
	path := ""
	if rt != nil && r.routes != nil && r.routes.multi() {
		path = rt.ID
	}
	for _, s := range r.segs {
		if !r.owned[s] && s.Remaining() > 0 {
			r.owned[s] = true
			s.Path = path
			return s
		}
	}
	var best *Segment
	for _, s := range r.segs {
		if r.owned[s] && (best == nil || s.Remaining() > best.Remaining()) {
			best = s
		}
	}
	if best == nil || best.Remaining() < 2*minSplit {
		return nil
	}
	pos := best.Start + best.Done
	mid := pos + best.Remaining()/2
	tail := &Segment{Start: mid, End: best.End, Path: path}
	best.End = mid - 1
	r.segs = append(r.segs, tail)
	r.owned[tail] = true
	return tail
}

func (r *runner) release(s *Segment) {
	r.mu.Lock()
	delete(r.owned, s)
	r.mu.Unlock()
}

func (r *runner) fail(err error) {
	r.errOnce.Do(func() {
		r.firstErr = err
		r.cancel()
	})
}

// fetchRange downloads the remaining bytes of seg, retrying transient errors.
func (r *runner) fetchRange(f *os.File, job *Job, seg *Segment, rt *routeState) error {
	maxRetries := r.m.settingsCopy().MaxRetries
	for attempt := 0; ; attempt++ {
		r.mu.Lock()
		from, end := seg.Start+seg.Done, seg.End
		r.mu.Unlock()
		if from > end {
			return nil
		}
		err := r.fetchOnce(f, job, seg, rt, from, end)
		if err == nil {
			return nil
		}
		if r.ctx.Err() != nil {
			return r.ctx.Err()
		}
		if errors.Is(err, errRangeIgnored) || attempt >= maxRetries || !retryable(err) {
			return err
		}
		if !sleepCtx(r.ctx, backoff(attempt, err)) {
			return r.ctx.Err()
		}
	}
}

func (r *runner) fetchOnce(f *os.File, job *Job, seg *Segment, rt *routeState, from, end int64) error {
	url := job.FinalURL
	if url == "" {
		url = job.URL
	}
	req, err := http.NewRequestWithContext(r.ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	applyHeaders(req, job.Headers)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", from, end))
	if job.ETag != "" {
		req.Header.Set("If-Range", job.ETag)
	}
	resp, err := rt.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		if resp.StatusCode == http.StatusOK {
			return errRangeIgnored
		}
		return &httpError{code: resp.StatusCode, status: resp.Status, retryAfter: resp.Header.Get("Retry-After")}
	}
	if start := contentRangeStart(resp.Header.Get("Content-Range")); start != from {
		return errRangeIgnored
	}
	return r.copyInto(f, resp.Body, seg, rt)
}

// copyInto writes body at the segment's cursor until the segment's (possibly
// shrinking) end is reached.
func (r *runner) copyInto(f *os.File, body io.Reader, seg *Segment, rt *routeState) error {
	buf := make([]byte, chunkSize)
	for {
		n, rerr := body.Read(buf)
		if n > 0 {
			if err := r.m.limiter.WaitN(r.ctx, n); err != nil {
				return err
			}
			r.mu.Lock()
			pos := seg.Start + seg.Done
			allowed := min(int64(n), seg.End-pos+1)
			r.mu.Unlock()
			if allowed > 0 {
				if _, err := f.WriteAt(buf[:allowed], pos); err != nil {
					r.fail(err) // disk errors are fatal for the whole job
					return err
				}
				r.mu.Lock()
				seg.Done += allowed
				finished := seg.Remaining() <= 0
				r.mu.Unlock()
				r.downloaded.Add(allowed)
				rt.bytes.Add(allowed)
				if finished {
					return nil
				}
			} else {
				return nil
			}
		}
		if rerr == io.EOF {
			r.mu.Lock()
			rem := seg.Remaining()
			r.mu.Unlock()
			if rem > 0 {
				return io.ErrUnexpectedEOF
			}
			return nil
		}
		if rerr != nil {
			return rerr
		}
	}
}

// stream downloads over a single connection for servers without range
// support or unknown sizes. It always restarts from zero.
func (r *runner) stream(f *os.File, job *Job) error {
	rt := r.routes.pick()
	if rt == nil {
		return errors.New("no working network route")
	}
	r.conns.Store(1)
	rt.conns.Store(1)
	defer func() {
		r.conns.Store(0)
		rt.conns.Store(0)
	}()
	maxRetries := r.m.settingsCopy().MaxRetries
	for attempt := 0; ; attempt++ {
		err := r.streamOnce(f, job, rt)
		if err == nil || r.ctx.Err() != nil || attempt >= maxRetries || !retryable(err) {
			if r.ctx.Err() != nil {
				return r.ctx.Err()
			}
			return err
		}
		if !sleepCtx(r.ctx, backoff(attempt, err)) {
			return r.ctx.Err()
		}
	}
}

func (r *runner) streamOnce(f *os.File, job *Job, rt *routeState) error {
	if err := f.Truncate(0); err != nil {
		return err
	}
	seg := &Segment{Start: 0, End: 1<<62 - 1}
	if job.Size > 0 {
		seg.End = job.Size - 1
	}
	r.mu.Lock()
	r.segs = []*Segment{seg}
	r.mu.Unlock()
	r.downloaded.Store(0)

	req, err := http.NewRequestWithContext(r.ctx, http.MethodGet, job.URL, nil)
	if err != nil {
		return err
	}
	applyHeaders(req, job.Headers)
	resp, err := rt.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &httpError{code: resp.StatusCode, status: resp.Status, retryAfter: resp.Header.Get("Retry-After")}
	}
	err = r.copyInto(f, resp.Body, seg, rt)
	if err == io.ErrUnexpectedEOF && job.Size <= 0 {
		err = nil // unknown length: EOF is the end
	}
	if err == nil && job.Size <= 0 {
		size := r.downloaded.Load()
		r.mu.Lock()
		seg.End = size - 1
		r.mu.Unlock()
		r.m.setSize(r.id, size)
	}
	return err
}

type httpError struct {
	code       int
	status     string
	retryAfter string
}

func (e *httpError) Error() string { return "server returned " + e.status }

func retryable(err error) bool {
	var he *httpError
	if errors.As(err, &he) {
		return he.code == 408 || he.code == 425 || he.code == 429 || he.code >= 500
	}
	return !errors.Is(err, context.Canceled)
}

func backoff(attempt int, err error) time.Duration {
	var he *httpError
	if errors.As(err, &he) && he.retryAfter != "" {
		if s, e := strconv.Atoi(he.retryAfter); e == nil && s > 0 && s < 300 {
			return time.Duration(s) * time.Second
		}
	}
	d := time.Duration(500<<min(attempt, 6)) * time.Millisecond // 0.5s .. 32s
	return d/2 + time.Duration(rand.Int64N(int64(d/2)+1))
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func contentRangeStart(v string) int64 {
	// bytes 100-199/1000
	v = strings.TrimPrefix(strings.TrimSpace(v), "bytes ")
	dash := strings.IndexByte(v, '-')
	if dash < 0 {
		return -1
	}
	n, err := strconv.ParseInt(v[:dash], 10, 64)
	if err != nil {
		return -1
	}
	return n
}

func joinPath(dir, name string) string { return filepath.Join(dir, name) }

// uniqueName returns name, or "name (n).ext" if it (or its .part) exists.
func uniqueName(dir, name string) string {
	exists := func(n string) bool {
		_, e1 := os.Stat(filepath.Join(dir, n))
		_, e2 := os.Stat(filepath.Join(dir, n+".part"))
		return e1 == nil || e2 == nil
	}
	if !exists(name) {
		return name
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	if strings.HasSuffix(strings.ToLower(base), ".tar") {
		ext = base[len(base)-4:] + ext
		base = base[:len(base)-4]
	}
	for i := 1; ; i++ {
		cand := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if !exists(cand) {
			return cand
		}
	}
}
