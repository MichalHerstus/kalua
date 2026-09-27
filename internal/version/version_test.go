package version

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

// stamp sets the link-time variables and restores them when the test ends.
func stamp(t *testing.T, v, commit, date string) {
	t.Helper()
	ov, oc, od := Version, Commit, Date
	t.Cleanup(func() { Version, Commit, Date = ov, oc, od })
	Version, Commit, Date = v, commit, date
}

func TestDefaultsAreDevBuild(t *testing.T) {
	// An unstamped `go build` keeps these defaults.
	if Version != "dev" || Commit != "unknown" || Date != "unknown" {
		t.Fatalf("package defaults = %q/%q/%q, want dev/unknown/unknown", Version, Commit, Date)
	}
	if !IsDev() {
		t.Error("IsDev() = false for the default stamp, want true")
	}
	got := Get()
	if got.Version != "dev" {
		t.Errorf("Get().Version = %q, want dev", got.Version)
	}
	if got.Stamped {
		t.Error("Get().Stamped = true for an unstamped build, want false")
	}
	if got.Go != runtime.Version() || got.OS != runtime.GOOS || got.Arch != runtime.GOARCH {
		t.Errorf("Get() platform = %q/%q/%q, want %q/%q/%q",
			got.Go, got.OS, got.Arch, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	}
}

func TestStringUnstampedDropsUnknownFields(t *testing.T) {
	stamp(t, "dev", "unknown", "unknown")
	got := Get().String()
	if strings.Contains(got, "unknown") {
		t.Errorf("String() = %q, should not print unknown fields", got)
	}
	if !strings.Contains(got, "unstamped") {
		t.Errorf("String() = %q, want it to mark the build unstamped", got)
	}
	if !strings.HasPrefix(got, "KALUA dev (") {
		t.Errorf("String() = %q, want prefix %q", got, "KALUA dev (")
	}
	if !strings.HasSuffix(got, runtime.GOOS+"/"+runtime.GOARCH+")") {
		t.Errorf("String() = %q, want it to end with the target platform", got)
	}
}

func TestStringStamped(t *testing.T) {
	stamp(t, "alfa", "c43b2be", "2026-09-25T19:27:50Z")
	got := Get().String()
	want := "KALUA alfa (c43b2be, built 2026-09-25T19:27:50Z, " +
		runtime.Version() + ", " + runtime.GOOS + "/" + runtime.GOARCH + ")"
	if got != want {
		t.Errorf("String() =\n  %q\nwant\n  %q", got, want)
	}
	if IsDev() {
		t.Error("IsDev() = true for a stamped build, want false")
	}
	if !Get().Stamped {
		t.Error("Get().Stamped = false for a stamped build, want true")
	}
}

func TestStringPartialStamp(t *testing.T) {
	// Only Version stamped: commit/date must be omitted, not rendered as noise.
	stamp(t, "alfa", "", "")
	got := Get().String()
	if strings.Contains(got, "unknown") || strings.Contains(got, "built") {
		t.Errorf("String() = %q, want commit/date omitted", got)
	}
	if !strings.HasPrefix(got, "KALUA alfa (") {
		t.Errorf("String() = %q, want prefix %q", got, "KALUA alfa (")
	}
}

func TestGetNormalizesBlankStamp(t *testing.T) {
	stamp(t, "  ", "  ", "")
	got := Get()
	if got.Version != "dev" {
		t.Errorf("Get().Version = %q, want dev for a blank stamp", got.Version)
	}
	if got.Commit != "unknown" || got.Date != "unknown" {
		t.Errorf("Get() commit/date = %q/%q, want unknown/unknown", got.Commit, got.Date)
	}
	if !IsDev() {
		t.Error("IsDev() = false for a blank stamp, want true")
	}
}

func TestInfoJSONShape(t *testing.T) {
	stamp(t, "alfa", "c43b2be", "2026-09-25T19:27:50Z")
	b, err := json.Marshal(Get())
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"version", "commit", "date", "go", "os", "arch", "stamped"} {
		if _, ok := m[k]; !ok {
			t.Errorf("JSON missing key %q; got %s", k, b)
		}
	}
	if m["version"] != "alfa" || m["commit"] != "c43b2be" || m["stamped"] != true {
		t.Errorf("JSON values = %s, want version/commit/stamped stamped", b)
	}
}
