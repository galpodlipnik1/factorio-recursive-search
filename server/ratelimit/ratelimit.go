package ratelimit

import (
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"rbf-api/applog"
	"rbf-api/httpx"
)

type Config struct {
	Logger            *applog.Logger
	Route             string
	RequestsPerMinute int
	Burst             int
	StaleAfter        time.Duration
	CleanupEvery      uint64
}

type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type Middleware struct {
	logger         *applog.Logger
	route          string
	limitPerMinute int
	burst          int
	staleAfter     time.Duration
	cleanupEvery   uint64
	visitors       map[string]*clientLimiter
	mu             sync.Mutex
	requestCount   atomic.Uint64
}

func New(cfg Config) func(http.Handler) http.Handler {
	requestsPerMinute := cfg.RequestsPerMinute
	if requestsPerMinute < 1 {
		requestsPerMinute = 1
	}

	burst := cfg.Burst
	if burst < 1 {
		burst = 1
	}

	staleAfter := cfg.StaleAfter
	if staleAfter <= 0 {
		staleAfter = 30 * time.Minute
	}

	cleanupEvery := cfg.CleanupEvery
	if cleanupEvery == 0 {
		cleanupEvery = 256
	}

	logger := cfg.Logger
	if logger == nil {
		logger = applog.NewForWriter(io.Discard)
	}

	middleware := &Middleware{
		logger:         logger,
		route:          cfg.Route,
		limitPerMinute: requestsPerMinute,
		burst:          burst,
		staleAfter:     staleAfter,
		cleanupEvery:   cleanupEvery,
		visitors:       make(map[string]*clientLimiter),
	}

	return middleware.Handler
}

func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := httpx.ClientIP(r)
		if ip == "" {
			ip = "unknown"
		}

		if m.requestCount.Add(1)%m.cleanupEvery == 0 {
			m.cleanup(time.Now())
		}

		allowed, retryAfterSeconds := m.allow(ip, time.Now())
		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds))
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)

			m.logger.Warn(
				applog.CategoryMiddleware,
				"rate limit exceeded",
				slog.String("ip", ip),
				slog.String("route", m.route),
				slog.String("method", r.Method),
				slog.Int("status", http.StatusTooManyRequests),
				slog.Int("limit_per_min", m.limitPerMinute),
				slog.Int("burst", m.burst),
				slog.Int("retry_after_s", retryAfterSeconds),
			)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (m *Middleware) allow(ip string, now time.Time) (bool, int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	visitor := m.visitors[ip]
	if visitor == nil {
		visitor = &clientLimiter{
			limiter: rate.NewLimiter(
				rate.Every(time.Minute/time.Duration(m.limitPerMinute)),
				m.burst,
			),
			lastSeen: now,
		}
		m.visitors[ip] = visitor
	}

	visitor.lastSeen = now

	reservation := visitor.limiter.ReserveN(now, 1)
	if !reservation.OK() {
		return false, 60
	}

	delay := reservation.DelayFrom(now)
	if delay > 0 {
		reservation.CancelAt(now)
		retryAfter := int(math.Ceil(delay.Seconds()))
		if retryAfter < 1 {
			retryAfter = 1
		}
		return false, retryAfter
	}

	return true, 0
}

func (m *Middleware) cleanup(now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for ip, visitor := range m.visitors {
		if now.Sub(visitor.lastSeen) > m.staleAfter {
			delete(m.visitors, ip)
		}
	}
}
