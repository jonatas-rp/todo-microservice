package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"todo-service/internal/logger"
)

// openAPI writes docs/openapi.json. The body is that document, not the JSON envelope.
// publicPrefix replaces the servers entry so Try it out stays on the ingress path.
func openAPI(w http.ResponseWriter, r *http.Request, publicPrefix string) {
	raw, err := readOpenAPI()
	if err != nil {
		logger.FromContext(r.Context()).Error("read openapi", map[string]any{"err": err.Error()})
		writeProblem(w, r, http.StatusInternalServerError, "internal error")
		return
	}
	if publicPrefix != "" {
		raw, err = withServerPrefix(raw, publicPrefix)
		if err != nil {
			logger.FromContext(r.Context()).Error("rewrite openapi servers", map[string]any{"err": err.Error()})
			writeProblem(w, r, http.StatusInternalServerError, "internal error")
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

func withServerPrefix(raw []byte, publicPrefix string) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	doc["servers"] = []any{
		map[string]any{"url": publicPrefix, "description": "Ingress"},
	}
	return json.Marshal(doc)
}

// readOpenAPI loads docs/openapi.json by walking up from the working directory.
func readOpenAPI() ([]byte, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	for {
		path := filepath.Join(dir, "docs", "openapi.json")
		raw, err := os.ReadFile(path) // #nosec G304
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
