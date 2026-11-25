package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

var (
	scriptPath string
	baseURL    string
)

func TestMain(m *testing.M) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Fprintln(os.Stderr, "resolve test file path")
		os.Exit(1)
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	scriptPath = filepath.Join(root, "scripts", "e2e.sh")

	if err := runScript("up"); err != nil {
		fmt.Fprintln(os.Stderr, "scripts/e2e.sh up:", err)
		os.Exit(1)
	}
	env, err := loadState(filepath.Join(root, "test", "e2e", ".state", "todo-service-e2e.env"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		_ = runScript("down")
		os.Exit(1)
	}
	baseURL = env["BASE_URL"]
	if baseURL == "" {
		fmt.Fprintln(os.Stderr, "BASE_URL missing from e2e state")
		_ = runScript("down")
		os.Exit(1)
	}

	code := m.Run()
	_ = runScript("down")
	os.Exit(code)
}

func TestAPIThroughContainer(t *testing.T) {
	key := uuid.NewString()
	rec := do(t, http.MethodPost, baseURL+"/v1/todos", `{"title":"buy milk","completed":false}`, map[string]string{
		"Content-Type":    "application/json",
		"Idempotency-Key": key,
	})
	if rec.status != http.StatusCreated {
		t.Fatalf("create status = %d body %s", rec.status, rec.body)
	}
	var created envelope[apiTodo]
	if err := json.Unmarshal(rec.body, &created); err != nil {
		t.Fatal(err)
	}
	if !created.Success || created.Data.Title != "buy milk" || created.Data.ID == "" {
		t.Fatalf("create = %+v", created)
	}
	if rec.header.Get("X-Request-ID") == "" {
		t.Fatal("missing X-Request-ID")
	}

	list := do(t, http.MethodGet, baseURL+"/v1/todos", "", nil)
	var listed envelope[[]apiTodo]
	if err := json.Unmarshal(list.body, &listed); err != nil {
		t.Fatal(err)
	}
	if list.status != http.StatusOK || len(listed.Data) != 1 || listed.Data[0].ID != created.Data.ID {
		t.Fatalf("list = %d %+v", list.status, listed)
	}

	replay := do(t, http.MethodPost, baseURL+"/v1/todos", `{"title":"other","completed":true}`, map[string]string{
		"Content-Type":    "application/json",
		"Idempotency-Key": key,
	})
	var again envelope[apiTodo]
	if err := json.Unmarshal(replay.body, &again); err != nil {
		t.Fatal(err)
	}
	if replay.status != http.StatusOK || again.Message != "todo already created" || again.Data.ID != created.Data.ID {
		t.Fatalf("replay = %d %+v", replay.status, again)
	}

	spec := do(t, http.MethodGet, baseURL+"/v1/openapi.json", "", nil)
	if spec.status != http.StatusOK {
		t.Fatalf("openapi = %d %s", spec.status, spec.body)
	}
	if !bytes.Contains(spec.body, []byte(`"openapi"`)) || !bytes.Contains(spec.body, []byte(`3.0.3`)) {
		t.Fatalf("openapi body = %s", spec.body)
	}

	docs := do(t, http.MethodGet, baseURL+"/v1/docs/", "", nil)
	if docs.status != http.StatusOK || !strings.Contains(docs.header.Get("Content-Type"), "text/html") {
		t.Fatalf("docs = %d %q %s", docs.status, docs.header.Get("Content-Type"), docs.body)
	}
	if !bytes.Contains(docs.body, []byte("swagger-ui")) {
		t.Fatalf("docs body missing swagger-ui: %s", docs.body)
	}
}

func TestContainerStopsOnSIGTERM(t *testing.T) {
	if rec := do(t, http.MethodGet, baseURL+"/livez", "", nil); rec.status != http.StatusOK {
		t.Fatalf("livez = %d %s", rec.status, rec.body)
	}

	out, err := scriptOutput("stop")
	if err != nil {
		t.Fatalf("scripts/e2e.sh stop: %v\n%s", err, out)
	}
	if !strings.Contains(out, "EXIT_CODE=0") {
		t.Fatalf("stop output = %q", out)
	}
	// Container is down; bring it back for any later tests and package cleanup.
	if err := runScript("up"); err != nil {
		t.Fatalf("scripts/e2e.sh up after stop: %v", err)
	}
	env, err := loadState(filepath.Join(filepath.Dir(scriptPath), "..", "test", "e2e", ".state", "todo-service-e2e.env"))
	if err != nil {
		t.Fatal(err)
	}
	baseURL = env["BASE_URL"]
}

func TestContainerInvalidPORT(t *testing.T) {
	cmd := exec.Command(scriptPath, "run-bad-port")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit, output %s", out)
	}
}

type apiTodo struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Completed bool   `json:"completed"`
}

type envelope[T any] struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type response struct {
	status int
	header http.Header
	body   []byte
}

func runScript(args ...string) error {
	cmd := exec.Command(scriptPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func scriptOutput(args ...string) (string, error) {
	cmd := exec.Command(scriptPath, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func loadState(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, "=") {
			continue
		}
		k, v, _ := strings.Cut(line, "=")
		out[k] = v
	}
	return out, nil
}

func do(t *testing.T, method, url, body string, headers map[string]string) response {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, url, r)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response{status: res.StatusCode, header: res.Header.Clone(), body: raw}
}
