package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

const DefaultUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0 Safari/537.36"

// Event names emitted to the frontend.
const (
	EventProgress = "progress"     // []Progress, a few times per second
	EventJob      = "job"          // Job, on any state change
	EventRemoved  = "jobs:removed" // []string
	EventBatch    = "batch"        // Batch
	EventBatchDel = "batch:removed"
	EventDone     = "job:done" // Job, completed
	EventSettings = "settings"
)

// State is a full snapshot for initial UI load.
type State struct {
	Jobs     []Job    `json:"jobs"`
	Batches  []Batch  `json:"batches"`
	Settings Settings `json:"settings"`
}

// AddRequest describes a single new download.
type AddRequest struct {
	URL         string            `json:"url"`
	Filename    string            `json:"filename"`
	Dir         string            `json:"dir"`
	Connections int               `json:"connections"`
	Headers     map[string]string `json:"headers"`
	Paused      bool              `json:"paused"`
	// Hints from a previous probe so the list looks right immediately.
	Size int64 `json:"size"`
}

// BatchRequest creates a batch plus its items.
type BatchRequest struct {
	Name        string       `json:"name"`
	Dir         string       `json:"dir"`
	MaxParallel int          `json:"maxParallel"`
	Sequential  bool         `json:"sequential"`
	Connections int          `json:"connections"`
	Paused      bool         `json:"paused"`
	Items       []AddRequest `json:"items"`
}

type speedState struct {
	last  int64
	at    time.Time
	speed float64
}

// Manager owns all jobs and schedules runners.
type Manager struct {
	mu       sync.Mutex
	jobs     map[string]*Job
	order    []string
	batches  map[string]*Batch
	settings Settings
	runners  map[string]*runner
	speeds   map[string]*speedState
	dirty    bool
	closing  bool

	client  *http.Client
	limiter *Limiter
	store   *Store
	emit    Emitter
	wake    chan struct{}
}

func DefaultSettings() Settings {
	dir := os.Getenv("ADM_DOWNLOAD_DIR")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, "Downloads")
	}
	return Settings{
		DownloadDir:        dir,
		MaxActive:          3,
		DefaultConnections: 8,
		MaxRetries:         6,
		PerHostLimit:       16,
		Theme:              "system",
		CategorizeByType:   false,
		NotifyOnComplete:   true,
	}
}

func NewManager(store *Store, emit Emitter) *Manager {
	if emit == nil {
		emit = nopEmitter{}
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 20 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     false, // separate TCP connections are the point of segmenting
		MaxIdleConns:          128,
		MaxIdleConnsPerHost:   32,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}
	return &Manager{
		jobs:     map[string]*Job{},
		batches:  map[string]*Batch{},
		runners:  map[string]*runner{},
		speeds:   map[string]*speedState{},
		settings: DefaultSettings(),
		client:   &http.Client{Transport: transport},
		limiter:  NewLimiter(0),
		store:    store,
		emit:     emit,
		wake:     make(chan struct{}, 1),
	}
}

// SetEmitter swaps the event sink (Wails only provides one after startup).
func (m *Manager) SetEmitter(e Emitter) {
	m.mu.Lock()
	m.emit = e
	m.mu.Unlock()
}

// Start loads persisted state and runs the background loops until ctx ends.
func (m *Manager) Start(ctx context.Context) error {
	if m.store != nil {
		st, err := m.store.Load()
		if err != nil {
			return err
		}
		m.mu.Lock()
		if st.Settings.MaxActive > 0 {
			m.settings = st.Settings
		}
		for i := range st.Jobs {
			j := st.Jobs[i]
			if j.Status.IsActive() {
				j.Status = StatusQueued // interrupted by shutdown: pick it back up
			}
			m.jobs[j.ID] = &j
			m.order = append(m.order, j.ID)
		}
		for i := range st.Batches {
			b := st.Batches[i]
			m.batches[b.ID] = &b
		}
		m.sortLocked()
		m.limiter.SetRate(m.settings.SpeedLimit)
		m.mu.Unlock()
	}
	go m.loop(ctx)
	m.kick()
	return nil
}

// Shutdown stops all runners and persists state.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	m.closing = true
	rs := make([]*runner, 0, len(m.runners))
	for id, r := range m.runners {
		rs = append(rs, r)
		m.jobs[id].Status = StatusQueued // resume on next launch
	}
	m.mu.Unlock()
	for _, r := range rs {
		r.cancel()
	}
	for _, r := range rs {
		select {
		case <-r.done:
		case <-time.After(3 * time.Second):
		}
	}
	m.persist()
}

func (m *Manager) loop(ctx context.Context) {
	progress := time.NewTicker(400 * time.Millisecond)
	save := time.NewTicker(time.Second)
	defer progress.Stop()
	defer save.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.wake:
			m.schedule()
		case <-progress.C:
			m.tickProgress()
		case <-save.C:
			m.mu.Lock()
			for id, r := range m.runners {
				m.syncRunnerLocked(id, r)
			}
			dirty := m.dirty || len(m.runners) > 0
			m.dirty = false
			m.mu.Unlock()
			if dirty {
				m.persist()
			}
		}
	}
}

func (m *Manager) kick() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *Manager) persist() {
	if m.store == nil {
		return
	}
	st := m.Snapshot()
	_ = m.store.Save(st)
}

// ---------- queries ----------

func (m *Manager) Snapshot() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := State{Settings: m.settings, Jobs: make([]Job, 0, len(m.order)), Batches: make([]Batch, 0, len(m.batches))}
	for _, id := range m.order {
		st.Jobs = append(st.Jobs, m.jobViewLocked(id))
	}
	for _, b := range m.batches {
		st.Batches = append(st.Batches, *b)
	}
	slices.SortFunc(st.Batches, func(a, b Batch) int { return int(a.CreatedAt - b.CreatedAt) })
	return st
}

func (m *Manager) jobViewLocked(id string) Job {
	j := *m.jobs[id]
	if r := m.runners[id]; r != nil {
		j.Segments = r.segments()
		j.Downloaded = r.downloaded.Load()
	} else {
		j.Segments = slices.Clone(j.Segments)
	}
	return j
}

func (m *Manager) jobCopy(id string) *Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if !ok {
		return nil
	}
	c := *j
	c.Segments = slices.Clone(j.Segments)
	return &c
}

func (m *Manager) settingsCopy() Settings {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.settings
}

func (m *Manager) Settings() Settings { return m.settingsCopy() }

// Job returns a snapshot of one job.
func (m *Manager) Job(id string) (Job, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.jobs[id]; !ok {
		return Job{}, false
	}
	return m.jobViewLocked(id), true
}

// ActiveCount is the number of jobs currently probing or downloading.
func (m *Manager) ActiveCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.runners)
}

// Client exposes the shared HTTP client (used for probing from the UI).
func (m *Manager) Client() *http.Client { return m.client }

// ---------- commands ----------

func (m *Manager) Add(req AddRequest) (Job, error) {
	m.mu.Lock()
	j, err := m.newJobLocked(req, "")
	if err != nil {
		m.mu.Unlock()
		return Job{}, err
	}
	m.dirty = true
	view := *j
	m.mu.Unlock()
	m.emit.Emit(EventJob, view)
	m.kick()
	return view, nil
}

func (m *Manager) AddBatch(req BatchRequest) (Batch, error) {
	if len(req.Items) == 0 {
		return Batch{}, errors.New("batch has no items")
	}
	m.mu.Lock()
	b := &Batch{
		ID:          newID(),
		Name:        strings.TrimSpace(req.Name),
		Dir:         req.Dir,
		MaxParallel: req.MaxParallel,
		Sequential:  req.Sequential,
		CreatedAt:   now(),
	}
	if b.Name == "" {
		b.Name = "Batch " + time.Now().Format("Jan 2 15:04")
	}
	if b.Dir == "" {
		b.Dir = m.settings.DownloadDir
	}
	if b.MaxParallel <= 0 {
		b.MaxParallel = m.settings.MaxActive
	}
	m.batches[b.ID] = b
	var views []Job
	for _, it := range req.Items {
		if it.Dir == "" {
			it.Dir = b.Dir
		}
		if it.Connections == 0 {
			it.Connections = req.Connections
		}
		it.Paused = it.Paused || req.Paused
		j, err := m.newJobLocked(it, b.ID)
		if err != nil {
			continue // skip invalid entries rather than failing the whole batch
		}
		views = append(views, *j)
	}
	if len(views) == 0 {
		delete(m.batches, b.ID)
		m.mu.Unlock()
		return Batch{}, errors.New("no valid URLs in batch")
	}
	m.dirty = true
	bv := *b
	m.mu.Unlock()
	m.emit.Emit(EventBatch, bv)
	for _, v := range views {
		m.emit.Emit(EventJob, v)
	}
	m.kick()
	return bv, nil
}

func (m *Manager) newJobLocked(req AddRequest, batchID string) (*Job, error) {
	u := strings.TrimSpace(req.URL)
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return nil, errors.New("only http and https URLs are supported")
	}
	dir := req.Dir
	if dir == "" {
		dir = m.settings.DownloadDir
	}
	conns := req.Connections
	if conns <= 0 {
		conns = m.settings.DefaultConnections
	}
	conns = min(conns, 32)
	name := SanitizeFilename(req.Filename)
	if strings.TrimSpace(req.Filename) == "" {
		name = ""
	}
	size := req.Size
	if size == 0 {
		size = -1
	}
	j := &Job{
		ID:          newID(),
		BatchID:     batchID,
		URL:         u,
		Filename:    name,
		Dir:         expandHome(dir),
		Category:    CategoryFor(name),
		Size:        size,
		Status:      StatusQueued,
		Connections: conns,
		Headers:     req.Headers,
		Position:    m.nextPositionLocked(),
		CreatedAt:   now(),
	}
	if name == "" {
		j.Filename = FilenameFromURLString(u)
		j.Category = CategoryFor(j.Filename)
	}
	if m.settings.CategorizeByType && req.Dir == "" && batchID == "" && j.Category != "other" {
		j.Dir = filepath.Join(j.Dir, strings.ToUpper(j.Category[:1])+j.Category[1:])
	}
	if req.Paused {
		j.Status = StatusPaused
	}
	m.jobs[j.ID] = j
	m.order = append(m.order, j.ID)
	return j, nil
}

func (m *Manager) nextPositionLocked() int {
	p := 0
	for _, j := range m.jobs {
		p = max(p, j.Position+1)
	}
	return p
}

// Pause stops the given jobs, keeping their progress.
func (m *Manager) Pause(ids []string) {
	m.mu.Lock()
	var stop []*runner
	var views []Job
	for _, id := range ids {
		j, ok := m.jobs[id]
		if !ok || j.Status == StatusCompleted || j.Status == StatusPaused {
			continue
		}
		if r := m.runners[id]; r != nil {
			stop = append(stop, r)
		}
		j.Status = StatusPaused
		views = append(views, m.jobViewLocked(id))
	}
	m.dirty = true
	m.mu.Unlock()
	for _, r := range stop {
		r.cancel()
	}
	for _, v := range views {
		m.emit.Emit(EventJob, v)
	}
}

// Resume re-queues paused or failed jobs.
func (m *Manager) Resume(ids []string) {
	m.mu.Lock()
	var views []Job
	for _, id := range ids {
		j, ok := m.jobs[id]
		if !ok || (j.Status != StatusPaused && j.Status != StatusFailed) {
			continue
		}
		// A runner that is still winding down will see Queued and leave it.
		j.Status = StatusQueued
		j.Error = ""
		views = append(views, *j)
	}
	m.dirty = true
	m.mu.Unlock()
	for _, v := range views {
		m.emit.Emit(EventJob, v)
	}
	m.kick()
}

// Remove deletes jobs from the list, optionally deleting files on disk.
func (m *Manager) Remove(ids []string, deleteFiles bool) {
	m.mu.Lock()
	var stop []*runner
	var paths []string
	removed := map[string]bool{}
	for _, id := range ids {
		j, ok := m.jobs[id]
		if !ok {
			continue
		}
		if r := m.runners[id]; r != nil {
			stop = append(stop, r)
		}
		if j.Status != StatusCompleted {
			paths = append(paths, j.PartPath())
		} else if deleteFiles {
			paths = append(paths, j.Path())
		}
		delete(m.jobs, id)
		removed[id] = true
	}
	m.order = slices.DeleteFunc(m.order, func(id string) bool { return removed[id] })
	// Drop batches that became empty.
	var emptyBatches []string
	for bid := range m.batches {
		if !m.batchHasJobsLocked(bid) {
			delete(m.batches, bid)
			emptyBatches = append(emptyBatches, bid)
		}
	}
	m.dirty = true
	m.mu.Unlock()
	for _, r := range stop {
		r.cancel()
		<-r.done
	}
	for _, p := range paths {
		_ = os.Remove(p)
	}
	m.emit.Emit(EventRemoved, ids)
	for _, b := range emptyBatches {
		m.emit.Emit(EventBatchDel, b)
	}
	m.kick()
}

func (m *Manager) batchHasJobsLocked(bid string) bool {
	for _, j := range m.jobs {
		if j.BatchID == bid {
			return true
		}
	}
	return false
}

// BatchJobIDs lists the jobs of a batch in order.
func (m *Manager) BatchJobIDs(batchID string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var ids []string
	for _, id := range m.order {
		if m.jobs[id].BatchID == batchID {
			ids = append(ids, id)
		}
	}
	return ids
}

// RetryFailed re-queues the failed jobs of a batch (or all failed jobs when
// batchID is empty).
func (m *Manager) RetryFailed(batchID string) {
	m.mu.Lock()
	var ids []string
	for _, id := range m.order {
		j := m.jobs[id]
		if j.Status == StatusFailed && (batchID == "" || j.BatchID == batchID) {
			ids = append(ids, id)
		}
	}
	m.mu.Unlock()
	m.Resume(ids)
}

// UpdateBatch changes batch-level scheduling options.
func (m *Manager) UpdateBatch(b Batch) {
	m.mu.Lock()
	cur, ok := m.batches[b.ID]
	if !ok {
		m.mu.Unlock()
		return
	}
	if b.Name != "" {
		cur.Name = b.Name
	}
	if b.MaxParallel > 0 {
		cur.MaxParallel = b.MaxParallel
	}
	cur.Sequential = b.Sequential
	m.dirty = true
	v := *cur
	m.mu.Unlock()
	m.emit.Emit(EventBatch, v)
	m.kick()
}

// UpdateSettings replaces settings and applies them live.
func (m *Manager) UpdateSettings(s Settings) Settings {
	m.mu.Lock()
	if s.MaxActive <= 0 {
		s.MaxActive = 1
	}
	if s.DefaultConnections <= 0 {
		s.DefaultConnections = 1
	}
	s.DefaultConnections = min(s.DefaultConnections, 32)
	if s.DownloadDir == "" {
		s.DownloadDir = m.settings.DownloadDir
	}
	s.DownloadDir = expandHome(s.DownloadDir)
	m.settings = s
	m.dirty = true
	m.mu.Unlock()
	m.limiter.SetRate(s.SpeedLimit)
	m.emit.Emit(EventSettings, s)
	m.kick()
	return s
}

// SetSpeedLimit is a shortcut used by the header profile switcher.
func (m *Manager) SetSpeedLimit(bps int64) {
	s := m.settingsCopy()
	s.SpeedLimit = bps
	m.UpdateSettings(s)
}

// Reorder moves the given job to a new position within the list.
func (m *Manager) Reorder(ids []string) {
	m.mu.Lock()
	for i, id := range ids {
		if j, ok := m.jobs[id]; ok {
			j.Position = i
		}
	}
	m.sortLocked()
	m.dirty = true
	m.mu.Unlock()
	m.kick()
}

func (m *Manager) sortLocked() {
	slices.SortStableFunc(m.order, func(a, b string) int { return m.jobs[a].Position - m.jobs[b].Position })
}

// ---------- scheduling ----------

func (m *Manager) schedule() {
	m.mu.Lock()
	if m.closing {
		m.mu.Unlock()
		return
	}
	active := len(m.runners)
	perBatch := map[string]int{}
	for id := range m.runners {
		if b := m.jobs[id].BatchID; b != "" {
			perBatch[b]++
		}
	}
	var start []Job
	for _, id := range m.order {
		if active >= m.settings.MaxActive {
			break
		}
		j := m.jobs[id]
		if j.Status != StatusQueued || m.runners[id] != nil {
			continue
		}
		if b := m.batches[j.BatchID]; b != nil {
			limit := b.MaxParallel
			if b.Sequential {
				limit = 1
			}
			if perBatch[b.ID] >= limit {
				continue
			}
			perBatch[b.ID]++
		}
		r := newRunner(m, id)
		m.runners[id] = r
		m.speeds[id] = &speedState{last: j.Downloaded, at: time.Now()}
		j.Status = StatusProbing
		j.Error = ""
		active++
		start = append(start, *j)
		go r.run()
	}
	m.mu.Unlock()
	for _, j := range start {
		m.emit.Emit(EventJob, j)
	}
}

// applyProbe stores probe results and flips the job to downloading.
func (m *Manager) applyProbe(id string, pj *Job, fresh bool) {
	m.mu.Lock()
	j, ok := m.jobs[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	j.Filename, j.Category = pj.Filename, pj.Category
	j.FinalURL, j.Size, j.Resumable, j.Probed = pj.FinalURL, pj.Size, pj.Resumable, true
	j.ETag, j.LastModified, j.ContentType = pj.ETag, pj.LastModified, pj.ContentType
	if fresh {
		j.Segments, j.Downloaded = nil, 0
	}
	if j.Status == StatusProbing {
		j.Status = StatusDownloading
	}
	m.dirty = true
	v := m.jobViewLocked(id)
	m.mu.Unlock()
	m.emit.Emit(EventJob, v)
}

func (m *Manager) setSize(id string, size int64) {
	m.mu.Lock()
	if j, ok := m.jobs[id]; ok {
		j.Size = size
	}
	m.mu.Unlock()
}

func (m *Manager) syncRunnerLocked(id string, r *runner) {
	if j, ok := m.jobs[id]; ok {
		j.Segments = r.segments()
		j.Downloaded = r.downloaded.Load()
	}
}

func (m *Manager) finishRunner(r *runner, err error) {
	m.mu.Lock()
	delete(m.runners, r.id)
	delete(m.speeds, r.id)
	j, ok := m.jobs[r.id]
	if !ok {
		m.mu.Unlock()
		m.kick()
		return
	}
	m.syncRunnerLocked(r.id, r)
	canceled := r.ctx.Err() != nil && (err == nil || errors.Is(err, context.Canceled))
	switch {
	case canceled:
		// Paused, removed or shutting down: status already set by the caller.
		if j.Status.IsActive() {
			j.Status = StatusPaused
		}
	case errors.Is(err, errRangeIgnored):
		j.SingleConn = true
		j.Segments, j.Downloaded = nil, 0
		j.Status = StatusQueued
	case err != nil:
		j.Status = StatusFailed
		j.Error = err.Error()
		j.Retries++
	default:
		if ferr := m.finalizeLocked(j); ferr != nil {
			j.Status = StatusFailed
			j.Error = ferr.Error()
		} else {
			j.Status = StatusCompleted
			j.CompletedAt = now()
			j.Downloaded = max(j.Downloaded, j.Size)
			j.Error = ""
		}
	}
	m.dirty = true
	v := m.jobViewLocked(r.id)
	m.mu.Unlock()
	m.emit.Emit(EventJob, v)
	if v.Status == StatusCompleted {
		m.emit.Emit(EventDone, v)
	}
	m.kick()
}

func (m *Manager) finalizeLocked(j *Job) error {
	part := j.PartPath()
	if _, err := os.Stat(j.Path()); err == nil {
		// Something appeared at the target while we were downloading.
		j.Filename = uniqueName(j.Dir, j.Filename)
	}
	return os.Rename(part, j.Path())
}

func (m *Manager) tickProgress() {
	m.mu.Lock()
	if len(m.runners) == 0 {
		m.mu.Unlock()
		return
	}
	t := time.Now()
	out := make([]Progress, 0, len(m.runners))
	for id, r := range m.runners {
		j := m.jobs[id]
		d := r.downloaded.Load()
		sp := m.speeds[id]
		if dt := t.Sub(sp.at).Seconds(); dt > 0 {
			inst := float64(d-sp.last) / dt
			if inst < 0 {
				inst = 0
			}
			if sp.speed == 0 {
				sp.speed = inst
			} else {
				sp.speed = 0.35*inst + 0.65*sp.speed
			}
		}
		sp.last, sp.at = d, t
		out = append(out, Progress{
			ID: id, Status: j.Status, Downloaded: d, Size: j.Size,
			Speed: sp.speed, Segments: r.segments(), Conns: int(r.conns.Load()),
		})
	}
	m.mu.Unlock()
	m.emit.Emit(EventProgress, out)
}

// ---------- helpers ----------

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") || p == "~" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	return p
}

// FilenameFromURLString guesses a filename before probing.
func FilenameFromURLString(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "download"
	}
	return FilenameFromResponse("", u, "")
}
