package coerce

import "fmt"

// stringifyOther falls back to Go's %v formatting for value kinds that never
// reach coerce today (they exist to keep Stringify open-ended).
func stringifyOther(v any) string { return fmt.Sprint(v) }
