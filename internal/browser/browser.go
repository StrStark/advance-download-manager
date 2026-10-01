// Package browser connects ADM to web browsers. A small extension catches
// downloads and hands them to ADM through native messaging: the browser
// starts `adm` as a "native host", which forwards the download to the running
// app (starting it if needed) and exits.
package browser

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// HostName is the native messaging host name the extension connects to.
const HostName = "io.github.adm"

// ChromeExtensionID is fixed by the "key" in the extension's manifest.
const ChromeExtensionID = "gogpglppbleogcdbdegcmohlaeocfbfi"

// FirefoxExtensionID is set in the Firefox manifest (browser_specific_settings).
const FirefoxExtensionID = "adm@strstark.github.io"

// Download is a download handed over by the browser.
type Download struct {
	URL       string `json:"url"`
	Filename  string `json:"filename,omitempty"`
	Referrer  string `json:"referrer,omitempty"`
	Cookies   string `json:"cookies,omitempty"`
	UserAgent string `json:"userAgent,omitempty"`
	Size      int64  `json:"size,omitempty"`
	Mime      string `json:"mime,omitempty"`
}

// Headers returns the request headers that make the server treat ADM like
// the browser session that started the download.
func (d Download) Headers() map[string]string {
	h := map[string]string{}
	if d.Referrer != "" {
		h["Referer"] = d.Referrer
	}
	if d.Cookies != "" {
		h["Cookie"] = d.Cookies
	}
	if d.UserAgent != "" {
		h["User-Agent"] = d.UserAgent
	}
	return h
}

// ---------- passing downloads between processes ----------

// ArgPrefix carries a download on the command line of a second ADM process;
// the single-instance lock hands those args to the running app.
const ArgPrefix = "--adm-download="

// EncodeArg packs d into a command-line argument.
func EncodeArg(d Download) string {
	b, _ := json.Marshal(d)
	return ArgPrefix + base64.RawURLEncoding.EncodeToString(b)
}

// DecodeArgs extracts downloads from command-line arguments.
func DecodeArgs(args []string) []Download {
	var out []Download
	for _, a := range args {
		raw, ok := strings.CutPrefix(a, ArgPrefix)
		if !ok {
			continue
		}
		b, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil {
			continue
		}
		var d Download
		if json.Unmarshal(b, &d) == nil && (strings.HasPrefix(d.URL, "http://") || strings.HasPrefix(d.URL, "https://")) {
			out = append(out, d)
		}
	}
	return out
}

// ---------- native messaging host ----------

// IsHostInvocation reports whether the browser started us as a native host.
// Chrome passes the caller origin ("chrome-extension://<id>/"); Firefox
// passes the manifest path and the extension ID.
func IsHostInvocation(args []string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, "chrome-extension://") || a == FirefoxExtensionID || a == "--native-messaging" {
			return true
		}
	}
	return false
}

type message struct {
	Type string `json:"type"` // "download" | "ping"
	Download
}

type reply struct {
	OK      bool   `json:"ok"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

// RunHost serves native messages on stdin/stdout until the browser closes
// the pipe. Each download is forwarded with forward (normally: launch ADM
// with the download as an argument).
func RunHost(in io.Reader, out io.Writer, version string, forward func(Download) error) error {
	for {
		msg, err := readMessage(in)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		var m message
		if err := json.Unmarshal(msg, &m); err != nil {
			writeMessage(out, reply{Error: "bad message"})
			continue
		}
		switch m.Type {
		case "ping":
			writeMessage(out, reply{OK: true, Version: version})
		case "download":
			if !strings.HasPrefix(m.URL, "http://") && !strings.HasPrefix(m.URL, "https://") {
				writeMessage(out, reply{Error: "only http(s) links are supported"})
				continue
			}
			if err := forward(m.Download); err != nil {
				writeMessage(out, reply{Error: err.Error()})
				continue
			}
			writeMessage(out, reply{OK: true, Version: version})
		default:
			writeMessage(out, reply{Error: "unknown message type"})
		}
	}
}

// Native messages are a 32-bit length (native byte order; little endian on
// every platform browsers run on) followed by UTF-8 JSON.
func readMessage(r io.Reader) ([]byte, error) {
	var n uint32
	if err := binary.Read(r, binary.LittleEndian, &n); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, io.EOF
		}
		return nil, err
	}
	if n > 4<<20 {
		return nil, errors.New("message too large")
	}
	b := make([]byte, n)
	_, err := io.ReadFull(r, b)
	return b, err
}

func writeMessage(w io.Writer, v any) error {
	b, _ := json.Marshal(v)
	if err := binary.Write(w, binary.LittleEndian, uint32(len(b))); err != nil {
		return err
	}
	_, err := w.Write(b)
	return err
}

// ForwardToApp launches ADM with the download as an argument. If ADM is
// already running, the single-instance lock passes it over and the new
// process exits at once; otherwise this starts ADM.
func ForwardToApp(d Download) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	cmd := exec.Command(exe, EncodeArg(d))
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// ---------- host registration ----------

// manifest is the native host description browsers read.
type manifest struct {
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	Path              string   `json:"path"`
	Type              string   `json:"type"`
	AllowedOrigins    []string `json:"allowed_origins,omitempty"`
	AllowedExtensions []string `json:"allowed_extensions,omitempty"`
}

func chromeManifest(exe string) manifest {
	return manifest{Name: HostName, Description: "ADM download manager", Path: exe, Type: "stdio",
		AllowedOrigins: []string{"chrome-extension://" + ChromeExtensionID + "/"}}
}

func firefoxManifest(exe string) manifest {
	return manifest{Name: HostName, Description: "ADM download manager", Path: exe, Type: "stdio",
		AllowedExtensions: []string{FirefoxExtensionID}}
}

func writeManifest(path string, m manifest) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	return os.WriteFile(path, b, 0o644)
}

// Register installs the native host manifests for every supported browser
// for the current user, pointing at this executable. It is idempotent and
// safe to call on every start (keeps the path right after updates).
func Register() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	return register(exe)
}
