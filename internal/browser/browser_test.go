package browser

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"testing"
)

func frame(v any) []byte {
	b, _ := json.Marshal(v)
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, uint32(len(b)))
	buf.Write(b)
	return buf.Bytes()
}

func readReplies(t *testing.T, r io.Reader) []reply {
	var out []reply
	for {
		b, err := readMessage(r)
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		var x reply
		json.Unmarshal(b, &x)
		out = append(out, x)
	}
}

func TestHostProtocol(t *testing.T) {
	var in bytes.Buffer
	in.Write(frame(map[string]any{"type": "ping"}))
	in.Write(frame(map[string]any{"type": "download", "url": "https://example.com/f.iso", "filename": "f.iso", "referrer": "https://example.com/", "cookies": "a=1"}))
	in.Write(frame(map[string]any{"type": "download", "url": "javascript:alert(1)"}))
	var out bytes.Buffer
	var got []Download
	err := RunHost(&in, &out, "1.2.3", func(d Download) error { got = append(got, d); return nil })
	if err != nil {
		t.Fatal(err)
	}
	rs := readReplies(t, &out)
	if len(rs) != 3 || !rs[0].OK || rs[0].Version != "1.2.3" || !rs[1].OK || rs[2].OK {
		t.Fatalf("replies: %+v", rs)
	}
	if len(got) != 1 || got[0].Cookies != "a=1" || got[0].Headers()["Referer"] != "https://example.com/" {
		t.Fatalf("forwarded: %+v", got)
	}
}

func TestArgRoundTrip(t *testing.T) {
	d := Download{URL: "https://x.test/a b.zip", Filename: "a b.zip", Cookies: "s=1; t=2", Size: 42}
	got := DecodeArgs([]string{"--background", EncodeArg(d), "https://plain.test/"})
	if len(got) != 1 || got[0] != d {
		t.Fatalf("got %+v", got)
	}
	if !IsHostInvocation([]string{"chrome-extension://" + ChromeExtensionID + "/"}) || !IsHostInvocation([]string{"/path/m.json", FirefoxExtensionID}) || IsHostInvocation([]string{"https://x"}) {
		t.Fatal("host detection")
	}
}

func TestRegisterLinux(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home+"/.config")
	if err := register("/usr/bin/adm"); err != nil {
		t.Skip(err) // non-Linux
	}
}
