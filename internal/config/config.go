// Package config provides the KALUA.INI configuration layer: a small INI
// parser plus helpers that combine CLI flags, KALUA.INI values and environment
// variables with the precedence: CLI flag > KALUA.INI > env var > default.
package config

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// File is a parsed KALUA.INI file. Section and key names are stored normalized,
// so lookups are case-insensitive and tolerant of '-' vs '_'.
type File struct {
	// sections maps normalized section name -> normalized key -> ordered values
	// (repeated keys append). order keeps the original section names in file
	// order for iteration (e.g. AI provider profile detection).
	sections map[string]map[string][]string
	order    []string
}

// New returns an empty configuration.
func New() *File {
	return &File{sections: map[string]map[string][]string{}}
}

// Norm normalizes a section or key name: lowercased with '-' and '_' removed,
// so "no-browser", "no_browser" and "NO_BROWSER" are all equivalent.
func Norm(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "_", "")
	return s
}

// stripBOM removes a leading UTF-8 byte-order mark.
func stripBOM(s string) string {
	return strings.TrimPrefix(s, "\ufeff")
}

// Parse parses INI text. ';' and '#' start a comment; a bare key without '='
// is treated as "true"; repeated keys append their values in file order.
func Parse(data []byte) *File {
	f := New()
	current := ""
	for _, raw := range strings.Split(stripBOM(string(data)), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			current = strings.TrimSpace(line[1 : len(line)-1])
			if _, ok := f.sections[Norm(current)]; !ok {
				f.sections[Norm(current)] = map[string][]string{}
				f.order = append(f.order, current)
			}
			continue
		}
		k, v := line, "true"
		if eq := strings.IndexByte(line, '='); eq >= 0 {
			k = strings.TrimSpace(line[:eq])
			v = strings.TrimSpace(line[eq+1:])
		}
		if k == "" {
			continue
		}
		sec := f.sections[Norm(current)]
		if sec == nil {
			sec = map[string][]string{}
			f.sections[Norm(current)] = sec
		}
		sec[Norm(k)] = append(sec[Norm(k)], v)
	}
	return f
}

// Load reads and parses the file at path.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data), nil
}

// FindPath resolves the KALUA.INI path: an explicit --ini flag wins, then the
// KALUA_INI env var, then ./KALUA.INI in the working directory. It returns ""
// when nothing is configured (the default file is absent).
func FindPath(iniFlag string) string {
	if p := strings.TrimSpace(iniFlag); p != "" {
		return p
	}
	if p := strings.TrimSpace(os.Getenv("KALUA_INI")); p != "" {
		return p
	}
	if _, err := os.Stat("KALUA.INI"); err == nil {
		return "KALUA.INI"
	}
	return ""
}

// Values returns all values recorded for key in section, in file order.
func (f *File) Values(section, key string) []string {
	if f == nil {
		return nil
	}
	return f.sections[Norm(section)][Norm(key)]
}

// Get returns the last value for key in section (INI "last wins").
func (f *File) Get(section, key string) (string, bool) {
	vals := f.Values(section, key)
	if len(vals) == 0 {
		return "", false
	}
	return vals[len(vals)-1], true
}

// GetAny returns the first of keys that is present in section.
func (f *File) GetAny(section string, keys ...string) (string, bool) {
	for _, k := range keys {
		if v, ok := f.Get(section, k); ok {
			return v, true
		}
	}
	return "", false
}

// SectionNames returns the original section names in file order.
func (f *File) SectionNames() []string {
	return f.order
}

// ApplyFlags fills KALUA.INI values into a parsed flag set for flags that were
// not explicitly set on the command line. aliases maps a flag name to extra INI
// key names to accept (e.g. "v" -> {"verbose"}). For each present value fs.Set
// is called once — repeated keys append for multi-value flags and "last value
// wins" for single-value flags. The first error is returned (e.g. an INI value
// the flag type rejects), with a message naming the key.
func (f *File) ApplyFlags(fs *flag.FlagSet, section string, aliases map[string][]string) error {
	if f == nil || fs == nil {
		return nil
	}
	sec := f.sections[Norm(section)]
	if sec == nil {
		return nil
	}
	explicit := map[string]bool{}
	fs.Visit(func(fl *flag.Flag) { explicit[fl.Name] = true })

	var firstErr error
	fs.VisitAll(func(fl *flag.Flag) {
		if firstErr != nil || explicit[fl.Name] || fl.Name == "ini" || fl.Name == "help" {
			return
		}
		keys := []string{fl.Name}
		keys = append(keys, aliases[fl.Name]...)
		var vals []string
		for _, k := range keys {
			vals = append(vals, sec[Norm(k)]...)
		}
		if len(vals) == 0 {
			return
		}
		for _, v := range vals {
			if _, ok := fl.Value.(boolFlag); ok {
				v = strconv.FormatBool(Truthy(v))
			}
			if err := fs.Set(fl.Name, v); err != nil {
				firstErr = fmt.Errorf("KALUA.INI [%s] %s=%q: %w", section, fl.Name, v, err)
				return
			}
		}
	})
	return firstErr
}

// Truthy interprets a KALUA.INI boolean value.
func Truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "t", "yes", "y", "on":
		return true
	}
	return false
}

// boolFlag mirrors flag.boolFlag so boolean flags can be recognized without
// reaching into the flag package internals.
type boolFlag interface {
	IsBoolFlag() bool
}
