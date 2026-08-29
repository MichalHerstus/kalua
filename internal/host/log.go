package host

import (
	"fmt"
	"io"
	"os"
	"sync"
)

type Level int

const (
	LevelError Level = iota
	LevelInfo
	LevelTrace
)

type Logger struct {
	mu     sync.Mutex
	out    io.Writer
	errOut io.Writer
	level  Level
}

func NewLogger(verbose bool) *Logger {
	l := &Logger{out: os.Stdout, errOut: os.Stderr, level: LevelInfo}
	if verbose {
		l.level = LevelTrace
	}
	return l
}

func (l *Logger) SetLevel(lv Level) { l.level = lv }

func (l *Logger) Printf(format string, args ...any) {
	if l.level >= LevelInfo {
		l.mu.Lock()
		fmt.Fprintf(l.out, format+"\n", args...)
		l.mu.Unlock()
	}
}

func (l *Logger) Tracef(format string, args ...any) {
	if l.level >= LevelTrace {
		l.mu.Lock()
		fmt.Fprintf(l.out, "[TRACE] "+format+"\n", args...)
		l.mu.Unlock()
	}
}

func (l *Logger) Errorf(format string, args ...any) {
	if l.level >= LevelError {
		l.mu.Lock()
		fmt.Fprintf(l.errOut, "[ERROR] "+format+"\n", args...)
		l.mu.Unlock()
	}
}

func (l *Logger) Warnf(format string, args ...any) {
	if l.level >= LevelInfo {
		l.mu.Lock()
		fmt.Fprintf(l.errOut, "[WARN] "+format+"\n", args...)
		l.mu.Unlock()
	}
}
