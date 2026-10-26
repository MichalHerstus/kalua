package cli

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// spinnerFrames is the animation sequence shown by the terminal spinner.
var spinnerFrames = []string{"|", "/", "-", "\\"}

// progressWriter renders live pipeline progress to a writer. On a terminal
// (character device) it shows an animated spinner with an elapsed timer that
// is cleared before each stage message; on any other writer (pipes, files,
// test buffers, CI logs) it writes stage messages as plain lines — readable
// output with no escape codes.
type progressWriter struct {
	w       io.Writer
	tty     bool
	mu      sync.Mutex // serializes writes to w (spinner goroutine vs. messages)
	began   time.Time
	stopCh  chan struct{}
	stopped bool
}

// newProgressWriter creates a progress renderer for w. TTY detection uses the
// character-device mode bit so there is no dependency on golang.org/x/term.
func newProgressWriter(w io.Writer) *progressWriter {
	tty := false
	if f, ok := w.(*os.File); ok {
		if fi, err := f.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
			tty = true
		}
	}
	return &progressWriter{w: w, tty: tty, began: time.Now()}
}

// start begins the spinner goroutine (terminal mode only); non-terminal
// writers are unaffected.
func (p *progressWriter) start() {
	if !p.tty {
		return
	}
	p.stopCh = make(chan struct{})
	go p.spin()
}

func (p *progressWriter) spin() {
	i := 0
	ticker := time.NewTicker(120 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.mu.Lock()
			if p.stopped {
				p.mu.Unlock()
				return
			}
			elapsed := time.Since(p.began).Round(100 * time.Millisecond)
			fmt.Fprintf(p.w, "\r%s %s", spinnerFrames[i%len(spinnerFrames)], elapsed)
			i++
			p.mu.Unlock()
		}
	}
}

// msg writes a stage message, clearing any spinner first in terminal mode.
// It is safe to call from any goroutine.
func (p *progressWriter) msg(text string) {
	if !p.tty {
		fmt.Fprintln(p.w, text)
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		fmt.Fprintln(p.w, text)
		return
	}
	// \x1b[2K erases the current line so a short message cannot leave
	// spinner characters behind; the message then starts on a fresh line.
	fmt.Fprint(p.w, "\r\x1b[2K")
	fmt.Fprintln(p.w, text)
}

// stop tears down the spinner (terminal mode), leaving the cursor on a clean
// line. It is safe to call more than once and from any goroutine.
func (p *progressWriter) stop() {
	if !p.tty {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		return
	}
	p.stopped = true
	close(p.stopCh)
	fmt.Fprint(p.w, "\r\x1b[2K")
}
