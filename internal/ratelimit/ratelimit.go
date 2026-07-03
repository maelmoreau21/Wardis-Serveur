package ratelimit

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type client struct {
	tokens     float64
	lastRefill time.Time
}

type Limiter struct {
	rate       float64 // tokens per second
	burst      float64 // max tokens
	mu         sync.Mutex
	ips        map[string]*client
	cleanupInt time.Duration
	stopCh     chan struct{}
}

func New(rate float64, burst float64) *Limiter {
	l := &Limiter{
		rate:       rate,
		burst:      burst,
		ips:        make(map[string]*client),
		cleanupInt: 10 * time.Minute,
		stopCh:     make(chan struct{}),
	}
	go l.cleanupLoop()
	return l
}

func (l *Limiter) Close() {
	close(l.stopCh)
}

func (l *Limiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := l.extractIP(r)

		if !l.allow(ip) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error": "too many requests"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (l *Limiter) extractIP(r *http.Request) string {
	// 1. Try X-Forwarded-For
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first valid IP in the list
		parts := strings.Split(xff, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if ip := net.ParseIP(part); ip != nil {
				return part
			}
		}
	}

	// 2. Fallback to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func (l *Limiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	c, exists := l.ips[ip]
	if !exists {
		c = &client{
			tokens:     l.burst,
			lastRefill: now,
		}
		l.ips[ip] = c
	}

	// Add tokens since last request
	elapsed := now.Sub(c.lastRefill).Seconds()
	c.lastRefill = now
	c.tokens += elapsed * l.rate
	if c.tokens > l.burst {
		c.tokens = l.burst
	}

	if c.tokens >= 1.0 {
		c.tokens -= 1.0
		return true
	}

	return false
}

func (l *Limiter) cleanupLoop() {
	ticker := time.NewTicker(l.cleanupInt)
	defer ticker.Stop()
	for {
		select {
		case <-l.stopCh:
			return
		case <-ticker.C:
			l.mu.Lock()
			now := time.Now()
			for ip, c := range l.ips {
				// Clean up clients that have not been seen for > 15 minutes and have full tokens
				if now.Sub(c.lastRefill) > 15*time.Minute && c.tokens >= l.burst {
					delete(l.ips, ip)
				}
			}
			l.mu.Unlock()
		}
	}
}
