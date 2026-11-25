package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"todo-service/internal/logger"
)

// openAPI writes docs/openapi.json. The body is that document, not the JSON envelope.
func openAPI(w http.ResponseWriter, r *http.Request) {
	raw, err := readOpenAPI()
	if err != nil {
		logger.FromContext(r.Context()).Error("read openapi", map[string]any{"err": err.Error()})
		writeProblem(w, r, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

// readOpenAPI loads docs/openapi.json by walking up from the working directory.
func readOpenAPI() ([]byte, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	for {
		path := filepath.Join(dir, "docs", "openapi.json")
		raw, err := os.ReadFile(path)
		if err == nil {
			return raw, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, fmt.Errorf("docs/openapi.json not found")
		}
		dir = parent
	}
}
