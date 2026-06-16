package handler

import (
	"net"
	"net/http"
	"sync"
	"time"
)

type rateLimitEntry struct {
	count int
	start time.Time
}

type loginRateLimiter struct {
	mu       sync.Mutex
	entries  map[string]*rateLimitEntry
	window   time.Duration
	maxHits  int
	cleanupInterval time.Duration
	stopCh   chan struct{}
}

func newLoginRateLimiter() *loginRateLimiter {
	rl := &loginRateLimiter{
		entries:  make(map[string]*rateLimitEntry),
		window:   15 * time.Minute,
		maxHits:  5,
		cleanupInterval: 5 * time.Minute,
		stopCh:   make(chan struct{}),
	}
	go rl.cleanupLoop()
	return rl
}

func (rl *loginRateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	entry, ok := rl.entries[ip]
	if !ok || now.Sub(entry.start) > rl.window {
		rl.entries[ip] = &rateLimitEntry{count: 1, start: now}
		return true
	}
	entry.count++
	return entry.count <= rl.maxHits
}

func (rl *loginRateLimiter) reset(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.entries, ip)
}

func (rl *loginRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()
			for ip, entry := range rl.entries {
				if now.Sub(entry.start) > rl.window {
					delete(rl.entries, ip)
				}
			}
			rl.mu.Unlock()
		case <-rl.stopCh:
			return
		}
	}
}

func (rl *loginRateLimiter) stop() {
	close(rl.stopCh)
}

func extractIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
