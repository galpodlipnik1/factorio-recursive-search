package httpx

import (
	"net"
	"net/http"
	"strings"
)

func ClientIP(r *http.Request) string {
	if forwarded := firstForwardedValue(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		return forwarded
	}

	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}

	return strings.TrimSpace(r.RemoteAddr)
}

func firstForwardedValue(value string) string {
	if value == "" {
		return ""
	}

	parts := strings.Split(value, ",")
	return strings.TrimSpace(parts[0])
}
