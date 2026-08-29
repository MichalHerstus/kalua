package server

import (
	"strconv"
	"sync"
)

// SharedState provides thread-safe shared key-value storage across workers.
type SharedState struct {
	mu   sync.RWMutex
	data map[string]string
}

// NewSharedState creates a new shared state store.
func NewSharedState() *SharedState {
	return &SharedState{data: make(map[string]string)}
}

// Set stores a key-value pair.
func (s *SharedState) Set(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
}

// Get retrieves a value by key. Returns empty string if not found.
func (s *SharedState) Get(key string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data[key]
}

// Del deletes a key.
func (s *SharedState) Del(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
}

// Keys returns all keys matching pattern (simple prefix match, * = all).
func (s *SharedState) Keys(pattern string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []string
	for k := range s.data {
		if pattern == "*" || pattern == "" || matchPrefix(k, pattern) {
			result = append(result, k)
		}
	}
	return result
}

// Incr increments a numeric value stored at key by delta.
// Returns the new value, or 0 if key doesn't exist or isn't numeric.
func (s *SharedState) Incr(key string, delta int64) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	valStr := s.data[key]
	var val int64
	if valStr != "" {
		// Parse integer
		parsed, err := strconv.ParseInt(valStr, 10, 64)
		if err == nil {
			val = parsed
		}
	}
	val += delta
	s.data[key] = strconv.FormatInt(val, 10)
	return val
}

// matchPrefix checks if key matches pattern (supports * wildcard at end).
func matchPrefix(key, pattern string) bool {
	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(key) >= len(prefix) && key[:len(prefix)] == prefix
	}
	return key == pattern
}
