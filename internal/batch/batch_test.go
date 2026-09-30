package batch

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestExpandPattern(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"http://x/img[1-3].jpg", []string{"http://x/img1.jpg", "http://x/img2.jpg", "http://x/img3.jpg"}},
		{"http://x/[08-10].png", []string{"http://x/08.png", "http://x/09.png", "http://x/10.png"}},
		{"http://x/p[0-10:5]", []string{"http://x/p0", "http://x/p5", "http://x/p10"}},
		{"http://x/[a-c]", []string{"http://x/a", "http://x/b", "http://x/c"}},
		{"http://x/[3-1]", []string{"http://x/3", "http://x/2", "http://x/1"}},
		{"http://x/s[1-2]e[01-02]", []string{"http://x/s1e01", "http://x/s1e02", "http://x/s2e01", "http://x/s2e02"}},
		{"http://x/plain", []string{"http://x/plain"}},
	}
	for _, c := range cases {
		got, err := ExpandPattern(c.in)
		if err != nil {
			t.Fatalf("%s: %v", c.in, err)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v want %v", c.in, got, c.want)
		}
	}
	if _, err := ExpandPattern("http://x/[1-100000]"); err == nil {
		t.Error("expected size limit error")
	}
}

func TestExtractURLs(t *testing.T) {
	text := `Grab these: https://a.com/file.zip, (https://b.org/x.iso).
	dupe https://a.com/file.zip
	pattern https://c.net/p[1-2].jpg
	[https://d.io/y.pdf] and junk ftp://nope`
	got := ExtractURLs(text)
	want := []string{"https://a.com/file.zip", "https://b.org/x.iso", "https://c.net/p1.jpg", "https://c.net/p2.jpg", "https://d.io/y.pdf"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
}

func TestParseFileCSVAndJSON(t *testing.T) {
	items, err := ParseFile("l.csv", strings.NewReader("url,filename,dir\nhttps://a/x.bin,renamed.bin,/tmp\nhttps://a/y.bin\n"))
	if err != nil || len(items) != 2 || items[0].Filename != "renamed.bin" || items[0].Dir != "/tmp" {
		t.Fatalf("csv: %v %+v", err, items)
	}
	items, err = ParseFile("l.json", strings.NewReader(`["https://a/1", "https://a/2"]`))
	if err != nil || len(items) != 2 {
		t.Fatalf("json: %v %+v", err, items)
	}
	items, err = ParseFile("l.json", strings.NewReader(`[{"url":"https://a/1","filename":"f"}]`))
	if err != nil || items[0].Filename != "f" {
		t.Fatalf("json objects: %v %+v", err, items)
	}
}

func TestRender(t *testing.T) {
	v := TemplateVars{Name: "clip", Ext: "mp4", Index: 7, Batch: "S1", Host: "cdn.x", Date: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)}
	cases := map[string]string{
		"":                                "clip.mp4",
		"{batch}_{index:03}_{name}.{ext}": "S1_007_clip.mp4",
		"{date}-{host}-{index}.{ext}":     "2026-09-30-cdn.x-7.mp4",
		"a/b{name}":                       "a_bclip",
	}
	for tpl, want := range cases {
		if got := Render(tpl, v); got != want {
			t.Errorf("%q: got %q want %q", tpl, got, want)
		}
	}
	if n, e := SplitName("x.tar.gz"); n != "x" || e != "tar.gz" {
		t.Errorf("SplitName: %q %q", n, e)
	}
}
