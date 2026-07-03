package ratelimit_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"wardis-server/internal/ratelimit"
)

func TestRateLimiter(t *testing.T) {
	// Limiter allowing 2 requests per second, burst of 2
	lim := ratelimit.New(2.0, 2.0)
	defer lim.Close()

	handler := lim.Limit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Helper to send a request from an IP
	sendReq := func(ip string) int {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = ip + ":12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}

	// 1. Initial burst of 2 requests should be allowed
	if code := sendReq("192.168.1.1"); code != http.StatusOK {
		t.Errorf("Expected 200 OK for first request, got %d", code)
	}
	if code := sendReq("192.168.1.1"); code != http.StatusOK {
		t.Errorf("Expected 200 OK for second request, got %d", code)
	}

	// 3rd request should be blocked immediately (burst is 2)
	if code := sendReq("192.168.1.1"); code != http.StatusTooManyRequests {
		t.Errorf("Expected 429 Too Many Requests for third request, got %d", code)
	}

	// 2. Different IP address should have its own separate limit
	if code := sendReq("192.168.1.2"); code != http.StatusOK {
		t.Errorf("Expected 200 OK for different IP address, got %d", code)
	}

	// 3. Wait for tokens to replenish (0.5s should replenish 1 token)
	time.Sleep(550 * time.Millisecond)

	if code := sendReq("192.168.1.1"); code != http.StatusOK {
		t.Errorf("Expected 200 OK after waiting for token replenishment, got %d", code)
	}
}

func TestXForwardedForIPExtraction(t *testing.T) {
	lim := ratelimit.New(1.0, 1.0)
	defer lim.Close()

	handler := lim.Limit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.195, 70.41.3.18, 150.172.238.178")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d", rec.Code)
	}
}
