package handler

import (
	_ "embed"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"rbf-api/applog"
	"rbf-api/httpx"
	"rbf-api/parser"
	"rbf-api/ratelimit"
)

const maxUploadSize = 128 << 20

//go:embed install-index.ps1.tmpl
var installIndexScriptTemplate string

type Config struct {
	InstallRateLimitPerMinute int
	InstallRateLimitBurst     int
	IndexRateLimitPerMinute   int
	IndexRateLimitBurst       int
}

func DefaultConfig() Config {
	return Config{
		InstallRateLimitPerMinute: 10,
		InstallRateLimitBurst:     3,
		IndexRateLimitPerMinute:   5,
		IndexRateLimitBurst:       2,
	}
}

func ConfigFromEnv() Config {
	cfg := DefaultConfig()
	cfg.InstallRateLimitPerMinute = envInt("RBF_INSTALL_RATE_LIMIT_PER_MINUTE", cfg.InstallRateLimitPerMinute)
	cfg.InstallRateLimitBurst = envInt("RBF_INSTALL_RATE_LIMIT_BURST", cfg.InstallRateLimitBurst)
	cfg.IndexRateLimitPerMinute = envInt("RBF_INDEX_RATE_LIMIT_PER_MINUTE", cfg.IndexRateLimitPerMinute)
	cfg.IndexRateLimitBurst = envInt("RBF_INDEX_RATE_LIMIT_BURST", cfg.IndexRateLimitBurst)
	return cfg
}

func NewRouter(logger *applog.Logger, cfg Config) http.Handler {
	router := chi.NewRouter()
	router.Use(chimiddleware.RequestID)
	router.Use(recoverer(logger))

	router.Get("/healthz", healthz)
	router.With(ratelimit.New(ratelimit.Config{
		Logger:            logger,
		Route:             "/install-index.ps1",
		RequestsPerMinute: cfg.InstallRateLimitPerMinute,
		Burst:             cfg.InstallRateLimitBurst,
	})).Get("/install-index.ps1", installIndexScript(logger))

	router.With(ratelimit.New(ratelimit.Config{
		Logger:            logger,
		Route:             "/index",
		RequestsPerMinute: cfg.IndexRateLimitPerMinute,
		Burst:             cfg.IndexRateLimitBurst,
	})).Post("/index", index(logger))

	return router
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}

func installIndexScript(logger *applog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		script := renderInstallIndexScript(r)

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if _, err := w.Write([]byte(script)); err != nil {
			logger.Error(
				applog.CategoryUsage,
				"failed to serve install script",
				append(
					requestAttrs(r, http.StatusOK, start),
					slog.String("error", err.Error()),
				)...,
			)
			return
		}

		logger.Info(
			applog.CategoryUsage,
			"served install script",
			requestAttrs(r, http.StatusOK, start)...,
		)
	}
}

func index(logger *applog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
		if err := r.ParseMultipartForm(maxUploadSize); err != nil {
			writeLoggedError(
				w,
				logger,
				r,
				start,
				http.StatusBadRequest,
				fmt.Sprintf("invalid multipart form: %v", err),
				err,
			)
			return
		}

		file, _, err := r.FormFile("blueprint_storage")
		if err != nil {
			writeLoggedError(
				w,
				logger,
				r,
				start,
				http.StatusBadRequest,
				"missing multipart field: blueprint_storage",
				err,
			)
			return
		}
		defer file.Close()

		data, err := io.ReadAll(file)
		if err != nil {
			writeLoggedError(
				w,
				logger,
				r,
				start,
				http.StatusBadRequest,
				fmt.Sprintf("failed to read upload: %v", err),
				err,
			)
			return
		}

		payload, err := parser.Build(data)
		if err != nil {
			writeLoggedError(
				w,
				logger,
				r,
				start,
				http.StatusUnprocessableEntity,
				fmt.Sprintf("failed to build index: %v", err),
				err,
			)
			return
		}

		module := parser.RenderLuaModule(payload)
		entryCount := len(payload.Entries)

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="index.lua"`)
		w.Header().Set("X-Rbf-Entry-Count", strconv.Itoa(entryCount))

		if _, err := w.Write([]byte(module)); err != nil {
			logger.Error(
				applog.CategoryUsage,
				"failed to write index response",
				append(
					requestAttrs(r, http.StatusOK, start),
					slog.Int("entry_count", entryCount),
					slog.String("error", err.Error()),
				)...,
			)
			return
		}

		logger.Info(
			applog.CategoryUsage,
			"built index",
			append(
				requestAttrs(r, http.StatusOK, start),
				slog.Int("entry_count", entryCount),
			)...,
		)
	}
}

func writeLoggedError(
	w http.ResponseWriter,
	logger *applog.Logger,
	r *http.Request,
	start time.Time,
	status int,
	responseMessage string,
	err error,
) {
	http.Error(w, responseMessage, status)

	attrs := requestAttrs(r, status, start)
	if err != nil {
		attrs = append(attrs, slog.String("error", err.Error()))
	}

	if status >= http.StatusInternalServerError {
		logger.Error(applog.CategoryUsage, responseMessage, attrs...)
		return
	}

	logger.Warn(applog.CategoryUsage, responseMessage, attrs...)
}

func requestAttrs(r *http.Request, status int, start time.Time) []slog.Attr {
	return []slog.Attr{
		slog.String("request_id", chimiddleware.GetReqID(r.Context())),
		slog.String("ip", httpx.ClientIP(r)),
		slog.String("route", r.URL.Path),
		slog.String("method", r.Method),
		slog.Int("status", status),
		slog.Int64("duration_ms", time.Since(start).Milliseconds()),
	}
}

func recoverer(logger *applog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			defer func() {
				recovered := recover()
				if recovered == nil {
					return
				}

				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

				logger.Error(
					applog.CategoryMiddleware,
					"panic recovered",
					append(
						requestAttrs(r, http.StatusInternalServerError, start),
						slog.String("error", fmt.Sprint(recovered)),
					)...,
				)
			}()

			next.ServeHTTP(w, r)
		})
	}
}

func renderInstallIndexScript(r *http.Request) string {
	apiURL := publicBaseURL(r) + "/index"
	return strings.ReplaceAll(
		installIndexScriptTemplate,
		"__DEFAULT_API_URL__",
		escapePowerShellSingleQuotedString(apiURL),
	)
}

func publicBaseURL(r *http.Request) string {
	scheme := firstForwardedValue(r.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}

	host := firstForwardedValue(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}

	return scheme + "://" + host
}

func firstForwardedValue(value string) string {
	if value == "" {
		return ""
	}

	parts := strings.Split(value, ",")
	return strings.TrimSpace(parts[0])
}

func escapePowerShellSingleQuotedString(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 1 {
		return fallback
	}

	return parsed
}
