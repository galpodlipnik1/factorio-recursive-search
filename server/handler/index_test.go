package handler

import (
	"bytes"
	"encoding/binary"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rbf-api/applog"
)

func TestInstallIndexScriptUsesRequestHostForDefaultAPIURL(t *testing.T) {
	router := NewRouter(applog.NewForWriter(io.Discard), DefaultConfig())

	request := httptest.NewRequest(http.MethodGet, "http://localhost:8080/install-index.ps1", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	body := recorder.Body.String()
	if !strings.Contains(body, "http://localhost:8080/index") {
		t.Fatalf("expected script to contain localhost index URL, body: %s", body)
	}
}

func TestInstallIndexScriptPrefersForwardedHeadersForDefaultAPIURL(t *testing.T) {
	router := NewRouter(applog.NewForWriter(io.Discard), DefaultConfig())

	request := httptest.NewRequest(http.MethodGet, "http://internal:8080/install-index.ps1", nil)
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Forwarded-Host", "rbf-api.example.com")

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	body := recorder.Body.String()
	if !strings.Contains(body, "https://rbf-api.example.com/index") {
		t.Fatalf("expected script to contain forwarded index URL, body: %s", body)
	}
}

func TestInstallIndexScriptIsRateLimited(t *testing.T) {
	cfg := DefaultConfig()
	cfg.InstallRateLimitPerMinute = 1
	cfg.InstallRateLimitBurst = 1

	router := NewRouter(applog.NewForWriter(io.Discard), cfg)

	firstRequest := httptest.NewRequest(http.MethodGet, "http://example.com/install-index.ps1", nil)
	firstRequest.RemoteAddr = "203.0.113.10:1000"
	firstResponse := httptest.NewRecorder()
	router.ServeHTTP(firstResponse, firstRequest)

	if firstResponse.Code != http.StatusOK {
		t.Fatalf("expected first request to pass, got %d", firstResponse.Code)
	}

	secondRequest := httptest.NewRequest(http.MethodGet, "http://example.com/install-index.ps1", nil)
	secondRequest.RemoteAddr = "203.0.113.10:1000"
	secondResponse := httptest.NewRecorder()
	router.ServeHTTP(secondResponse, secondRequest)

	if secondResponse.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second request to be rate limited, got %d", secondResponse.Code)
	}

	if secondResponse.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header to be set")
	}
}

func TestIndexRouteIsRateLimited(t *testing.T) {
	cfg := DefaultConfig()
	cfg.IndexRateLimitPerMinute = 1
	cfg.IndexRateLimitBurst = 1

	router := NewRouter(applog.NewForWriter(io.Discard), cfg)

	firstRequest := httptest.NewRequest(http.MethodPost, "http://example.com/index", nil)
	firstRequest.RemoteAddr = "203.0.113.20:1000"
	firstResponse := httptest.NewRecorder()
	router.ServeHTTP(firstResponse, firstRequest)

	if firstResponse.Code != http.StatusBadRequest {
		t.Fatalf("expected first request to reach handler and fail validation, got %d", firstResponse.Code)
	}

	secondRequest := httptest.NewRequest(http.MethodPost, "http://example.com/index", nil)
	secondRequest.RemoteAddr = "203.0.113.20:1000"
	secondResponse := httptest.NewRecorder()
	router.ServeHTTP(secondResponse, secondRequest)

	if secondResponse.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second request to be rate limited, got %d", secondResponse.Code)
	}
}

func TestIndexRouteRejectsMalformedMultipart(t *testing.T) {
	router := NewRouter(applog.NewForWriter(io.Discard), DefaultConfig())
	request := httptest.NewRequest(http.MethodPost, "http://example.com/index", strings.NewReader("--broken"))
	request.Header.Set("Content-Type", "multipart/form-data; boundary=broken")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Code)
	}
	if !strings.Contains(response.Body.String(), "invalid multipart form") {
		t.Fatalf("expected invalid multipart error, body: %s", response.Body.String())
	}
}

func TestIndexRouteRejectsMissingBlueprintStorageField(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("unrelated", "value"); err != nil {
		t.Fatalf("write unrelated multipart field: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	router := NewRouter(applog.NewForWriter(io.Discard), DefaultConfig())
	request := httptest.NewRequest(http.MethodPost, "http://example.com/index", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Code)
	}
	if !strings.Contains(response.Body.String(), "missing multipart field: blueprint_storage") {
		t.Fatalf("expected missing blueprint_storage error, body: %s", response.Body.String())
	}
}

func TestIndexRouteRejectsUnsupportedStorageVersion(t *testing.T) {
	request := newBlueprintStorageRequest(t, testStorageVersion(2, 1, 0, 0))
	response := httptest.NewRecorder()
	router := NewRouter(applog.NewForWriter(io.Discard), DefaultConfig())

	router.ServeHTTP(response, request)

	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status %d, got %d", http.StatusUnprocessableEntity, response.Code)
	}
	if !strings.Contains(response.Body.String(), "unsupported_version") {
		t.Fatalf("expected unsupported_version error, body: %s", response.Body.String())
	}
}

func TestIndexRouteRejectsTruncatedStorage(t *testing.T) {
	request := newBlueprintStorageRequest(t, []byte{2, 0})
	response := httptest.NewRecorder()
	router := NewRouter(applog.NewForWriter(io.Discard), DefaultConfig())

	router.ServeHTTP(response, request)

	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status %d, got %d", http.StatusUnprocessableEntity, response.Code)
	}
	if !strings.Contains(response.Body.String(), "truncation") {
		t.Fatalf("expected truncation error, body: %s", response.Body.String())
	}
}

func TestIndexRouteReturnsDeterministicLuaForMinimalStorage(t *testing.T) {
	storage := minimalBlueprintStorage()

	firstResponse := serveBlueprintStorage(t, storage)
	secondResponse := serveBlueprintStorage(t, storage)

	if firstResponse.Code != http.StatusOK {
		t.Fatalf("expected first status %d, got %d: %s", http.StatusOK, firstResponse.Code, firstResponse.Body.String())
	}
	if secondResponse.Code != http.StatusOK {
		t.Fatalf("expected second status %d, got %d: %s", http.StatusOK, secondResponse.Code, secondResponse.Body.String())
	}
	if got := firstResponse.Header().Get("X-Rbf-Entry-Count"); got != "1" {
		t.Fatalf("expected first X-Rbf-Entry-Count header 1, got %q", got)
	}
	if got := secondResponse.Header().Get("X-Rbf-Entry-Count"); got != "1" {
		t.Fatalf("expected second X-Rbf-Entry-Count header 1, got %q", got)
	}
	if got := firstResponse.Header().Get("Content-Disposition"); got != `attachment; filename="index.lua"` {
		t.Fatalf("expected index.lua attachment, got %q", got)
	}
	if !strings.Contains(firstResponse.Body.String(), "schema_version = 2") {
		t.Fatalf("expected schema version in response body: %s", firstResponse.Body.String())
	}
	if !strings.Contains(firstResponse.Body.String(), `path_key = "1"`) {
		t.Fatalf("expected root path key in response body: %s", firstResponse.Body.String())
	}
	if !strings.Contains(firstResponse.Body.String(), `name = "Minimal"`) {
		t.Fatalf("expected blueprint name in response body: %s", firstResponse.Body.String())
	}
	if firstResponse.Body.String() != secondResponse.Body.String() {
		t.Fatal("expected deterministic response body for identical storage")
	}
}

func serveBlueprintStorage(t *testing.T, storage []byte) *httptest.ResponseRecorder {
	t.Helper()

	router := NewRouter(applog.NewForWriter(io.Discard), DefaultConfig())
	request := newBlueprintStorageRequest(t, storage)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func newBlueprintStorageRequest(t *testing.T, storage []byte) *http.Request {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("blueprint_storage", "blueprint-storage-2.dat")
	if err != nil {
		t.Fatalf("create blueprint_storage multipart field: %v", err)
	}
	written, err := part.Write(storage)
	if err != nil {
		t.Fatalf("write blueprint storage: %v", err)
	}
	if written != len(storage) {
		t.Fatalf("expected to write %d storage bytes, wrote %d", len(storage), written)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "http://example.com/index", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func minimalBlueprintStorage() []byte {
	content := testStorageVersion(2, 0, 76, 0)
	content = append(content, 0)                   // version separator
	content = append(content, 0)                   // migrations
	content = append(content, make([]byte, 20)...) // reserved 2.x blueprint fields
	content = appendTestString(content, "Minimal description")
	content = append(content, 0) // no snap-to-grid
	content = binary.LittleEndian.AppendUint32(content, 0)

	blueprint := appendTestString(nil, "Minimal")
	blueprint = append(blueprint, 0) // blueprint separator
	blueprint = append(blueprint, 0) // no local prototype index
	blueprint = appendTestCount(blueprint, uint32(len(content)))
	blueprint = append(blueprint, content...)

	storage := testStorageVersion(2, 0, 76, 0)
	storage = append(storage, 0)                           // version separator
	storage = append(storage, 0)                           // migrations
	storage = binary.LittleEndian.AppendUint16(storage, 0) // no prototype groups
	storage = append(storage, 0)                           // library state
	storage = append(storage, 0)                           // library-state separator
	storage = binary.LittleEndian.AppendUint32(storage, 1)
	storage = binary.LittleEndian.AppendUint32(storage, 2)
	storage = binary.LittleEndian.AppendUint32(storage, 0) // reserved 2.x header field
	storage = append(storage, 1)                           // library marker
	storage = binary.LittleEndian.AppendUint32(storage, 4) // one live slot plus three previews
	storage = append(storage, 1, 0)                        // occupied blueprint slot
	storage = binary.LittleEndian.AppendUint32(storage, 1) // generation
	storage = binary.LittleEndian.AppendUint16(storage, 0) // default blueprint prototype
	storage = append(storage, blueprint...)
	storage = append(storage, 0, 0, 0) // unoccupied preview slots
	storage = append(storage, 0, 0, 0) // reserved footer
	return storage
}

func testStorageVersion(major uint16, minor uint16, patch uint16, build uint16) []byte {
	data := make([]byte, 0, 8)
	data = binary.LittleEndian.AppendUint16(data, major)
	data = binary.LittleEndian.AppendUint16(data, minor)
	data = binary.LittleEndian.AppendUint16(data, patch)
	return binary.LittleEndian.AppendUint16(data, build)
}

func appendTestString(data []byte, value string) []byte {
	data = appendTestCount(data, uint32(len(value)))
	return append(data, value...)
}

func appendTestCount(data []byte, value uint32) []byte {
	if value < 0xff {
		return append(data, byte(value))
	}

	data = append(data, 0xff)
	return binary.LittleEndian.AppendUint32(data, value)
}
