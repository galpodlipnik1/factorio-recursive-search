package handler

import (
	"fmt"
	"io"
	"net/http"

	"rbf-api/parser"
)

const maxUploadSize = 128 << 20

func NewMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/index", index)
	return mux
}

func index(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		http.Error(w, fmt.Sprintf("invalid multipart form: %v", err), http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("blueprint_storage")
	if err != nil {
		http.Error(w, "missing multipart field: blueprint_storage", http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read upload: %v", err), http.StatusBadRequest)
		return
	}

	exportStrings := parser.ExtractBlueprintStrings(data)
	if len(exportStrings) == 0 {
		http.Error(w, "no valid blueprint strings found in upload", http.StatusUnprocessableEntity)
		return
	}

	module, entryCount, err := parser.BuildModule(exportStrings)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to build index: %v", err), http.StatusUnprocessableEntity)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="index.lua"`)
	w.Header().Set("X-Rbf-Entry-Count", fmt.Sprintf("%d", entryCount))
	_, _ = w.Write([]byte(module))
}
