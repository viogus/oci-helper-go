package handler

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type rateLimitEntry struct {
	count int
	start time.Time
}

type loginRateLimiter struct {
	mu              sync.Mutex
	entries         map[string]*rateLimitEntry
	blockedIPs      map[string]time.Time
	window          time.Duration
	maxHits         int
	cleanupInterval time.Duration
	stopCh          chan struct{}
}

func newLoginRateLimiter() *loginRateLimiter {
	rl := &loginRateLimiter{
		entries:         make(map[string]*rateLimitEntry),
		blockedIPs:      make(map[string]time.Time),
		window:          15 * time.Minute,
		maxHits:         5,
		cleanupInterval: 5 * time.Minute,
		stopCh:          make(chan struct{}),
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
	if entry.count > rl.maxHits {
		rl.blockedIPs[ip] = now
		return false
	}
	return true
}

func (rl *loginRateLimiter) reset(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.entries, ip)
}

func (rl *loginRateLimiter) isBlocked(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Check persistent blacklist first.
	if _, blocked := rl.blockedIPs[ip]; blocked {
		return true
	}
	// Check time-window rate limit.
	entry, ok := rl.entries[ip]
	if ok && time.Since(entry.start) <= rl.window && entry.count > rl.maxHits {
		return true
	}
	return false
}

func (rl *loginRateLimiter) clearBlockedIP(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if ip == "" {
		// Clear all.
		rl.blockedIPs = make(map[string]time.Time)
		return true
	}
	_, exists := rl.blockedIPs[ip]
	delete(rl.blockedIPs, ip)
	return exists
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
			for ip, blockedAt := range rl.blockedIPs {
				if now.Sub(blockedAt) > rl.window {
					delete(rl.blockedIPs, ip)
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
	// Check X-Forwarded-For from trusted proxies first.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" && isTrustedProxy(r.RemoteAddr) {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	// Fall back to X-Real-IP from trusted proxies.
	if xri := r.Header.Get("X-Real-IP"); xri != "" && isTrustedProxy(r.RemoteAddr) {
		return strings.TrimSpace(xri)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
