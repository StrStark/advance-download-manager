// Package batch turns user input (pasted text, files, URL patterns) into
// download items, and renders rename templates.
package batch

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// MaxItems caps pattern expansion and imports so a typo can't create a
// million jobs.
const MaxItems = 10000

// Item is one entry of a batch before it becomes a job.
type Item struct {
	URL      string `json:"url"`
	Filename string `json:"filename,omitempty"`
	Dir      string `json:"dir,omitempty"`
}

var urlRe = regexp.MustCompile(`https?://[^\s<>"'` + "`" + `]+`)

// ExtractURLs finds every http(s) URL in free text, trims trailing
// punctuation, and removes duplicates while keeping order. Lines that contain
// a range pattern like [1-10] are expanded.
func ExtractURLs(text string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(u string) {
		if !seen[u] && len(out) < MaxItems {
			seen[u] = true
			out = append(out, u)
		}
	}
	for _, m := range urlRe.FindAllString(text, -1) {
		m = strings.TrimRight(m, ".,;:!?)}'\"")
		// Keep "]" only when it closes a bracket inside the URL (patterns, IPv6).
		for strings.HasSuffix(m, "]") && strings.Count(m, "]") > strings.Count(m, "[") {
			m = strings.TrimRight(m[:len(m)-1], ".,;:!?)}'\"")
		}
		if HasPattern(m) {
			if exp, err := ExpandPattern(m); err == nil {
				for _, e := range exp {
					add(e)
				}
				continue
			}
		}
		add(m)
	}
	return out
}

var patternRe = regexp.MustCompile(`\[([0-9]+|[a-zA-Z])-([0-9]+|[a-zA-Z])(?::([0-9]+))?\]`)

// HasPattern reports whether s contains at least one [a-b] range.
func HasPattern(s string) bool { return patternRe.MatchString(s) }

// ExpandPattern expands numeric ranges ([1-10], [001-250], [0-100:5]) and
// letter ranges ([a-z]). Multiple ranges produce the cartesian product.
// Zero padding follows the width of the start value.
func ExpandPattern(p string) ([]string, error) {
	locs := patternRe.FindAllStringSubmatchIndex(p, -1)
	if len(locs) == 0 {
		return []string{p}, nil
	}
	type rng struct{ vals []string }
	var ranges []rng
	var lits []string
	prev := 0
	total := 1
	for _, l := range locs {
		lits = append(lits, p[prev:l[0]])
		prev = l[1]
		from, to := p[l[2]:l[3]], p[l[4]:l[5]]
		step := 1
		if l[6] >= 0 {
			step, _ = strconv.Atoi(p[l[6]:l[7]])
			if step <= 0 {
				return nil, fmt.Errorf("step must be positive")
			}
		}
		vals, err := expandRange(from, to, step)
		if err != nil {
			return nil, err
		}
		total *= len(vals)
		if total > MaxItems {
			return nil, fmt.Errorf("pattern expands to more than %d URLs", MaxItems)
		}
		ranges = append(ranges, rng{vals})
	}
	lits = append(lits, p[prev:])

	out := make([]string, 0, total)
	idx := make([]int, len(ranges))
	for {
		var b strings.Builder
		for i, r := range ranges {
			b.WriteString(lits[i])
			b.WriteString(r.vals[idx[i]])
		}
		b.WriteString(lits[len(lits)-1])
		out = append(out, b.String())
		// odometer increment, rightmost range fastest
		k := len(idx) - 1
		for k >= 0 {
			idx[k]++
			if idx[k] < len(ranges[k].vals) {
				break
			}
			idx[k] = 0
			k--
		}
		if k < 0 {
			return out, nil
		}
	}
}

func expandRange(from, to string, step int) ([]string, error) {
	fa, fn := strconv.Atoi(from)
	ta, tn := strconv.Atoi(to)
	var out []string
	switch {
	case fn == nil && tn == nil:
		width := 0
		if len(from) > 1 && from[0] == '0' {
			width = len(from)
		}
		inc := step
		if ta < fa {
			inc = -step
		}
		for v := fa; (inc > 0 && v <= ta) || (inc < 0 && v >= ta); v += inc {
			if len(out) >= MaxItems {
				return nil, fmt.Errorf("range too large")
			}
			out = append(out, fmt.Sprintf("%0*d", width, v))
		}
	case fn != nil && tn != nil && utf8.RuneCountInString(from) == 1 && utf8.RuneCountInString(to) == 1:
		a, b := rune(from[0]), rune(to[0])
		inc := rune(step)
		if b < a {
			inc = -inc
		}
		for c := a; (inc > 0 && c <= b) || (inc < 0 && c >= b); c += inc {
			out = append(out, string(c))
		}
	default:
		return nil, fmt.Errorf("invalid range [%s-%s]", from, to)
	}
	return out, nil
}

// ParseFile reads a list of items from a .txt, .csv or .json file.
func ParseFile(name string, r io.Reader) ([]Item, error) {
	data, err := io.ReadAll(io.LimitReader(r, 32<<20))
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".csv":
		return parseCSV(string(data))
	case ".json":
		return parseJSON(data)
	default:
		var items []Item
		for _, u := range ExtractURLs(string(data)) {
			items = append(items, Item{URL: u})
		}
		return items, nil
	}
}

// parseCSV accepts "url[,filename[,dir]]" rows with an optional header.
func parseCSV(s string) ([]Item, error) {
	cr := csv.NewReader(strings.NewReader(s))
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	var items []Item
	for i, row := range rows {
		if len(row) == 0 {
			continue
		}
		u := strings.TrimSpace(row[0])
		if i == 0 && !strings.HasPrefix(u, "http") {
			continue // header
		}
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			continue
		}
		it := Item{URL: u}
		if len(row) > 1 {
			it.Filename = strings.TrimSpace(row[1])
		}
		if len(row) > 2 {
			it.Dir = strings.TrimSpace(row[2])
		}
		items = append(items, it)
		if len(items) >= MaxItems {
			break
		}
	}
	return items, nil
}

// parseJSON accepts ["url", ...] or [{"url":..., "filename":..., "dir":...}].
func parseJSON(b []byte) ([]Item, error) {
	var strs []string
	if err := json.Unmarshal(b, &strs); err == nil {
		items := make([]Item, 0, len(strs))
		for _, s := range strs {
			items = append(items, Item{URL: strings.TrimSpace(s)})
		}
		return items, nil
	}
	var items []Item
	if err := json.Unmarshal(b, &items); err != nil {
		return nil, errors.New("JSON must be an array of URLs or {url, filename, dir} objects")
	}
	return items, nil
}

var tokenRe = regexp.MustCompile(`\{(name|ext|index|batch|date|host)(?::(0\d+))?\}`)

// TemplateVars are the values available to a rename template.
type TemplateVars struct {
	Name  string // original filename without extension
	Ext   string // extension without dot
	Index int    // 1-based position in batch
	Batch string
	Host  string
	Date  time.Time
}

// Render expands a template like "{batch}_{index:03}_{name}.{ext}".
// An empty template returns the original filename.
func Render(tpl string, v TemplateVars) string {
	if strings.TrimSpace(tpl) == "" {
		if v.Ext == "" {
			return v.Name
		}
		return v.Name + "." + v.Ext
	}
	out := tokenRe.ReplaceAllStringFunc(tpl, func(tok string) string {
		m := tokenRe.FindStringSubmatch(tok)
		switch m[1] {
		case "name":
			return v.Name
		case "ext":
			return v.Ext
		case "index":
			if m[2] != "" {
				w, _ := strconv.Atoi(m[2])
				return fmt.Sprintf("%0*d", w, v.Index)
			}
			return strconv.Itoa(v.Index)
		case "batch":
			return v.Batch
		case "date":
			return v.Date.Format("2006-01-02")
		case "host":
			return v.Host
		}
		return tok
	})
	out = strings.TrimSuffix(out, ".") // "{name}.{ext}" with no ext
	return strings.ReplaceAll(out, "/", "_")
}

// SplitName splits "archive.tar.gz" into ("archive", "tar.gz").
func SplitName(filename string) (name, ext string) {
	lower := strings.ToLower(filename)
	for _, double := range []string{".tar.gz", ".tar.xz", ".tar.bz2", ".tar.zst"} {
		if strings.HasSuffix(lower, double) {
			return filename[:len(filename)-len(double)], filename[len(filename)-len(double)+1:]
		}
	}
	e := path.Ext(filename)
	if e == "" || len(e) > 10 {
		return filename, ""
	}
	return strings.TrimSuffix(filename, e), e[1:]
}
