package core

import "time"

// Status is the lifecycle state of a job.
type Status string

const (
	StatusQueued      Status = "queued"
	StatusProbing     Status = "probing"
	StatusDownloading Status = "downloading"
	StatusPaused      Status = "paused"
	StatusCompleted   Status = "completed"
	StatusFailed      Status = "failed"
)

// IsActive reports whether the job currently owns a runner.
func (s Status) IsActive() bool { return s == StatusProbing || s == StatusDownloading }

// Segment is an inclusive byte range [Start, End] of which Done bytes have
// been written starting at Start.
type Segment struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
	Done  int64 `json:"done"`
	// Path is the connection route (network link / proxy) currently or last
	// used for this segment; empty for the default route.
	Path string `json:"path,omitempty"`
}

func (s Segment) Remaining() int64 { return s.End - s.Start + 1 - s.Done }

// Job is a single file download. Jobs that belong to a batch carry BatchID.
type Job struct {
	ID           string            `json:"id"`
	BatchID      string            `json:"batchId,omitempty"`
	URL          string            `json:"url"`
	FinalURL     string            `json:"finalUrl,omitempty"`
	Filename     string            `json:"filename"`
	Dir          string            `json:"dir"`
	Category     string            `json:"category"`
	Size         int64             `json:"size"` // -1 when unknown
	Downloaded   int64             `json:"downloaded"`
	Status       Status            `json:"status"`
	Error        string            `json:"error,omitempty"`
	Resumable    bool              `json:"resumable"`
	SingleConn   bool              `json:"singleConn,omitempty"` // server lies about ranges
	Probed       bool              `json:"probed"`
	ETag         string            `json:"etag,omitempty"`
	LastModified string            `json:"lastModified,omitempty"`
	ContentType  string            `json:"contentType,omitempty"`
	Connections  int               `json:"connections"`
	Segments     []Segment         `json:"segments"`
	Headers      map[string]string `json:"headers,omitempty"`
	// Proxy is "" (use the default), "direct", "system", or a profile ID.
	Proxy       string `json:"proxy,omitempty"`
	Position    int    `json:"position"`
	Retries     int    `json:"retries"`
	CreatedAt   int64  `json:"createdAt"`
	CompletedAt int64  `json:"completedAt,omitempty"`
}

// Path returns the final destination path.
func (j *Job) Path() string { return joinPath(j.Dir, j.Filename) }

// PartPath returns the in-progress path.
func (j *Job) PartPath() string { return j.Path() + ".part" }

// Batch groups jobs that are managed as one unit.
type Batch struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Dir         string `json:"dir"`
	MaxParallel int    `json:"maxParallel"`
	Sequential  bool   `json:"sequential"`
	CreatedAt   int64  `json:"createdAt"`
}

// Settings are user preferences that affect the engine.
type Settings struct {
	DownloadDir        string `json:"downloadDir"`
	MaxActive          int    `json:"maxActive"`
	DefaultConnections int    `json:"defaultConnections"`
	SpeedLimit         int64  `json:"speedLimit"` // bytes/s, 0 = unlimited
	MaxRetries         int    `json:"maxRetries"`
	PerHostLimit       int    `json:"perHostLimit"`
	UserAgent          string `json:"userAgent"`
	Theme              string `json:"theme"` // system | dark | light
	CategorizeByType   bool   `json:"categorizeByType"`
	ClipboardWatch     bool   `json:"clipboardWatch"`
	NotifyOnComplete   bool   `json:"notifyOnComplete"`

	// Proxies. DefaultProxy is "" (direct), "system" (environment), or a
	// profile ID. ProxyBypass lists hosts that always go direct.
	Proxies       []ProxyProfile `json:"proxies"`
	Subscriptions []Subscription `json:"subscriptions"`
	DefaultProxy  string         `json:"defaultProxy"`
	ProxyBypass   []string       `json:"proxyBypass"`

	// MultiLink spreads each download over several network connections
	// (e.g. Ethernet + a phone hotspot). Links limits it to these link IDs;
	// empty means every connected link.
	MultiLink bool     `json:"multiLink"`
	Links     []string `json:"links"`

	// Desktop integration.
	Autostart     bool `json:"autostart"`     // start at login (only set with the user's consent)
	Onboarded     bool `json:"onboarded"`     // the first-run questions were answered
	NoUpdateCheck bool `json:"noUpdateCheck"` // don't look for new versions automatically
}

// ProxyProfile is one proxy server. URL is either a proxy URL
// (http://, https://, socks5://) or a V2Ray/Xray share link
// (vmess://, vless://, trojan://, ss://).
type ProxyProfile struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Type           string `json:"type"` // http | https | socks5 | vmess | vless | trojan | shadowsocks
	URL            string `json:"url"`
	Server         string `json:"server,omitempty"` // host:port, for display
	SubscriptionID string `json:"subscriptionId,omitempty"`
}

// Subscription is a URL that serves a list of share links.
type Subscription struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	UpdatedAt int64  `json:"updatedAt"`
}

// PathStat is live per-route throughput for one download.
type PathStat struct {
	ID    string  `json:"id"`
	Label string  `json:"label"`
	Kind  string  `json:"kind"`
	Speed float64 `json:"speed"`
	Conns int     `json:"conns"`
}

// Progress is the light-weight, high-frequency update sent to the UI.
type Progress struct {
	ID         string     `json:"id"`
	Status     Status     `json:"status"`
	Downloaded int64      `json:"downloaded"`
	Size       int64      `json:"size"`
	Speed      float64    `json:"speed"` // bytes/s, smoothed
	Segments   []Segment  `json:"segments"`
	Conns      int        `json:"conns"` // live connections
	Paths      []PathStat `json:"paths,omitempty"`
}

// Emitter delivers engine events to whatever frontend is attached.
type Emitter interface {
	Emit(event string, data any)
}

type nopEmitter struct{}

func (nopEmitter) Emit(string, any) {}

func now() int64 { return time.Now().UnixMilli() }
