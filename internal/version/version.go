// Package version exposes KALUA build metadata.
//
// The three variables below are stamped into the binary at link time:
//
//	go build -ldflags "-X kalua/internal/version.Version=alfa \
//	                   -X kalua/internal/version.Commit=c43b2be \
//	                   -X kalua/internal/version.Date=2026-09-25T19:27:50Z"
//
// `make build` and `make dist` derive all three from git; .goreleaser.yml feeds
// the same variables from its own template context. An unstamped `go build`
// keeps the defaults below and is reported as a development build, so a missing
// stamp is visible rather than silently wrong.
package version

import (
	"runtime"
	"strings"
)

// unknown marks a build field that was never stamped.
const unknown = "unknown"

// devVersion is the Version reported by unstamped builds.
const devVersion = "dev"

// Stamped at link time with -ldflags -X. Keep the names in sync with the
// -X flags in the Makefile and .goreleaser.yml.
var (
	Version = devVersion
	Commit  = unknown
	Date    = unknown
)

// Info is the structured build metadata reported by `KALUA version --json`.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
	Go      string `json:"go"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	Stamped bool   `json:"stamped"`
}

// Get returns the build metadata of the running binary.
func Get() Info {
	return Info{
		Version: orDev(Version),
		Commit:  orUnknown(Commit),
		Date:    orUnknown(Date),
		Go:      runtime.Version(),
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
		Stamped: !IsDev(),
	}
}

// IsDev reports whether the binary carries no version stamp, i.e. it was built
// by a bare `go build` rather than `make build` / `make dist` / goreleaser.
func IsDev() bool {
	v := strings.TrimSpace(Version)
	return v == "" || v == devVersion
}

// String renders the one-line banner, e.g.
//
//	KALUA alfa (c43b2be, built 2026-09-25T19:27:50Z, go1.26.3, darwin/arm64)
//	KALUA dev (unstamped, go1.26.3, darwin/arm64)
//
// Unstamped fields are dropped rather than printed as "unknown".
func (i Info) String() string {
	details := make([]string, 0, 4)
	if i.Commit != "" && i.Commit != unknown {
		details = append(details, i.Commit)
	}
	if i.Date != "" && i.Date != unknown {
		details = append(details, "built "+i.Date)
	}
	if !i.Stamped {
		details = append(details, "unstamped")
	}
	details = append(details, i.Go, i.OS+"/"+i.Arch)
	return "KALUA " + i.Version + " (" + strings.Join(details, ", ") + ")"
}

func orDev(v string) string {
	if s := strings.TrimSpace(v); s != "" {
		return s
	}
	return devVersion
}

func orUnknown(v string) string {
	if s := strings.TrimSpace(v); s != "" {
		return s
	}
	return unknown
}
