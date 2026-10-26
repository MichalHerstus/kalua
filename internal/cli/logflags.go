package cli

import (
	"fmt"
	"time"

	"kalua/internal/klog"
)

// sizeValue adapts a klog.ParseSize expression ("16k", "50", "1.5M") to a
// flag.Value holding bytes.
type sizeValue struct {
	value *int64
}

func (s *sizeValue) String() string {
	if s == nil || s.value == nil {
		return ""
	}
	return fmt.Sprintf("%d", *s.value)
}

func (s *sizeValue) Set(raw string) error {
	n, err := klog.ParseSize(raw)
	if err != nil {
		return err
	}
	*s.value = n
	return nil
}

// durValue adapts a klog.ParseRetention expression ("7d", "24h", "0") to a
// flag.Value holding a time.Duration.
type durValue struct {
	value *time.Duration
}

func (d *durValue) String() string {
	if d == nil || d.value == nil {
		return ""
	}
	return d.value.String()
}

func (d *durValue) Set(raw string) error {
	rv, err := klog.ParseRetention(raw)
	if err != nil {
		return err
	}
	*d.value = rv
	return nil
}
