package ratelimit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type KeyFunc func(r *http.Request) (key string, ok bool)

// RedisFixedWindowLimiter is a distributed fixed-window rate limiter:
// limit requests per window per derived key (ip/user/apiKey/etc).
type RedisFixedWindowLimiter struct {
	RDB        *redis.Client
	Prefix     string        // e.g. "rl"
	Limit      int64         // e.g. 30
	Window     time.Duration // e.g. time.Minute
	KeyFunc    KeyFunc
	FailOpen   bool // if Redis is down, allow requests (true) or block (false)
	AddHeaders bool // add X-RateLimit-* headers
}

// NewRedisFixedWindowLimiter with sane defaults.
func NewRedisFixedWindowLimiter(rdb *redis.Client, limit int64, window time.Duration, keyFn KeyFunc) *RedisFixedWindowLimiter {
	return &RedisFixedWindowLimiter{
		RDB:        rdb,
		Prefix:     "rl",
		Limit:      limit,
		Window:     window,
		KeyFunc:    keyFn,
		FailOpen:   true,
		AddHeaders: true,
	}
}

// lua script makes INCR + initial EXPIRE atomic and returns:
// [count, ttl_ms]
var fixedWindowLua = redis.NewScript(`
local current = redis.call("INCR", KEYS[1])
if current == 1 then
  redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
local ttl = redis.call("PTTL", KEYS[1])
return {current, ttl}
`)

// Middleware returns an http middleware that enforces the limit.
func (l *RedisFixedWindowLimiter) Middleware(next http.Handler) http.Handler {
	if l.KeyFunc == nil {
		panic("RedisFixedWindowLimiter: KeyFunc must not be nil")
	}
	if l.Window <= 0 {
		panic("RedisFixedWindowLimiter: Window must be > 0")
	}
	if l.Limit <= 0 {
		panic("RedisFixedWindowLimiter: Limit must be > 0")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keyPart, ok := l.KeyFunc(r)
		if !ok || keyPart == "" {
			// If we can't derive a key, you can either:
			// - allow (safer for UX)
			// - or treat as a single shared bucket (safer for abuse)
			// We'll allow by default.
			next.ServeHTTP(w, r)
			return
		}

		redisKey := l.makeRedisKey(keyPart, l.Window)
		ctx, cancel := context.WithTimeout(r.Context(), 150*time.Millisecond)
		defer cancel()

		res, err := fixedWindowLua.Run(ctx, l.RDB, []string{redisKey}, l.Window.Milliseconds()).Result()
		if err != nil {
			if l.FailOpen {
				next.ServeHTTP(w, r)
				return
			}
			http.Error(w, "rate limiter unavailable", http.StatusServiceUnavailable)
			return
		}

		// Parse lua return: {count, ttl_ms}
		arr, _ := res.([]any)
		if len(arr) != 2 {
			if l.FailOpen {
				next.ServeHTTP(w, r)
				return
			}
			http.Error(w, "rate limiter unavailable", http.StatusServiceUnavailable)
			return
		}

		count := toInt64(arr[0])
		ttlMs := toInt64(arr[1])

		if l.AddHeaders {
			w.Header().Set("X-RateLimit-Limit", strconv.FormatInt(l.Limit, 10))
			w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(maxInt64(0, l.Limit-count), 10))
			// reset in seconds (approx)
			if ttlMs > 0 {
				w.Header().Set("X-RateLimit-Reset", strconv.FormatInt((ttlMs+999)/1000, 10))
			}
		}

		if count > l.Limit {
			// Retry-After is in seconds
			retryAfter := int64(1)
			if ttlMs > 0 {
				retryAfter = (ttlMs + 999) / 1000
				if retryAfter < 1 {
					retryAfter = 1
				}
			}
			w.Header().Set("Retry-After", strconv.FormatInt(retryAfter, 10))
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (l *RedisFixedWindowLimiter) makeRedisKey(keyPart string, window time.Duration) string {
	// Window bucket: unix_time / window_seconds
	now := time.Now().Unix()
	winSec := int64(window.Seconds())
	if winSec < 1 {
		winSec = 1
	}
	bucket := now / winSec

	// Avoid huge keys / weird chars by hashing keyPart (optional but nice).
	sum := sha256.Sum256([]byte(keyPart))
	keyHash := hex.EncodeToString(sum[:])

	return fmt.Sprintf("%s:%d:%s", l.Prefix, bucket, keyHash)
}

func toInt64(v any) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case string:
		n, _ := strconv.ParseInt(t, 10, 64)
		return n
	default:
		return 0
	}
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

//
// Key funcs you can reuse
//

// KeyByIP rate-limits by remote IP (good baseline).
func KeyByIP(r *http.Request) (string, bool) {
	ip := clientIP(r)
	if ip == "" {
		return "", false
	}
	return "ip:" + ip, true
}

// KeyByHeader uses a header value as identity (e.g. "X-Voter-Id" or "Authorization").
// If you're using bearer tokens, consider hashing the token before using it directly.
func KeyByHeader(header string) KeyFunc {
	return func(r *http.Request) (string, bool) {
		v := strings.TrimSpace(r.Header.Get(header))
		if v == "" {
			return "", false
		}
		return strings.ToLower(header) + ":" + v, true
	}
}

// KeyByIPOrHeader uses header if present, otherwise falls back to IP.
func KeyByIPOrHeader(header string) KeyFunc {
	hf := KeyByHeader(header)
	return func(r *http.Request) (string, bool) {
		if k, ok := hf(r); ok {
			return k, true
		}
		return KeyByIP(r)
	}
}

// clientIP tries common proxy headers then falls back to RemoteAddr.
// If you deploy behind a proxy, make sure you trust these headers (or have the proxy overwrite them).
func clientIP(r *http.Request) string {
	// X-Forwarded-For may contain multiple IPs: client, proxy1, proxy2...
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xrip != "" {
		if net.ParseIP(xrip) != nil {
			return xrip
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && net.ParseIP(host) != nil {
		return host
	}
	if net.ParseIP(r.RemoteAddr) != nil {
		return r.RemoteAddr
	}
	return ""
}
