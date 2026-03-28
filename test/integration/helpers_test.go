package integration

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"prs/internal/api"
	"prs/internal/cleanup"
	"prs/internal/config"
	"prs/internal/recovery"
	"prs/internal/store"
	"prs/internal/worker"
)

type testStaticFS struct{}

func (testStaticFS) Open(name string) (http.File, error) {
	return os.Open("testdata/static/" + name)
}

type testEnv struct {
	server  *httptest.Server
	store   *store.Store
	queue   *worker.Queue
	pool    *worker.Pool
	cleaner *cleanup.Retention
	dataDir string
	cfg     *config.Config
}

func newTestEnvFull(t *testing.T, maxBytes int64, numWorkers int) *testEnv {
	t.Helper()

	dataDir := t.TempDir()

	cfg := &config.Config{
		DataDir:             dataDir,
		MaxUploadBytes:      maxBytes,
		BaseURL:             "http://test",
		Workers:             numWorkers,
		RetentionDays:       5,
		CleanupInterval:     time.Hour,
		DiskExpansionFactor: 1.5,
	}

	s := store.New(cfg.DataDir)
	toEnqueue, err := recovery.Run(cfg.DataDir, s)
	if err != nil {
		t.Fatalf("recovery: %v", err)
	}

	q := worker.NewQueue(1000)
	proc := worker.NewProcessor(cfg.DataDir, cfg.DiskExpansionFactor, s)
	pool := worker.NewPool(q, proc)

	for _, id := range toEnqueue {
		q.Enqueue(id)
	}
	if numWorkers > 0 {
		pool.Start(numWorkers)
	}

	cleaner := cleanup.New(cfg.DataDir, cfg.RetentionDays, cfg.CleanupInterval, s)

	hcfg := api.HandlerConfig{
		DataDir:        cfg.DataDir,
		MaxUploadBytes: cfg.MaxUploadBytes,
		BaseURL:        cfg.BaseURL,
		Workers:        cfg.Workers,
	}
	h := api.New(hcfg, s, q, pool, testStaticFS{})
	router := api.NewRouter(h)

	srv := httptest.NewServer(router)
	t.Cleanup(func() {
		srv.Close()
		if numWorkers > 0 {
			pool.Stop()
		}
		cleaner.Stop()
	})

	return &testEnv{
		server:  srv,
		store:   s,
		queue:   q,
		pool:    pool,
		cleaner: cleaner,
		dataDir: dataDir,
		cfg:     cfg,
	}
}

func newTestEnv(t *testing.T) *testEnv {
	return newTestEnvFull(t, 100<<20, 1)
}

func newTestEnvWithMaxBytes(t *testing.T, maxBytes int64) *testEnv {
	return newTestEnvFull(t, maxBytes, 1)
}

func newTestEnvWithWorkers(t *testing.T, numWorkers int) *testEnv {
	return newTestEnvFull(t, 100<<20, numWorkers)
}

// buildZip creates a minimal valid ZIP in memory.
func buildZip(files map[string]string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := zw.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := io.WriteString(f, content); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// uploadZip posts a ZIP to /reports and returns the response.
func uploadZip(t *testing.T, env *testEnv, zipData []byte) *http.Response {
	t.Helper()
	return uploadBytes(t, env, zipData, "report.zip")
}

// uploadRaw posts raw bytes (not necessarily a valid ZIP) to /reports.
func uploadRaw(t *testing.T, env *testEnv, data []byte) *http.Response {
	t.Helper()
	return uploadBytes(t, env, data, "report.zip")
}

func uploadBytes(t *testing.T, env *testEnv, data []byte, filename string) *http.Response {
	t.Helper()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	mw.Close()

	req, err := http.NewRequest(http.MethodPost, env.server.URL+"/reports", &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// waitForStatus polls GET /reports/:id/status until the status matches or timeout.
func waitForStatus(t *testing.T, env *testEnv, id, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(env.server.URL + "/reports/" + id + "/status")
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			resp.Body.Close()
			t.Fatal(err)
		}
		resp.Body.Close()
		if body["status"] == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for status %q on report %s", want, id)
}

func decodeJSON(t *testing.T, r io.Reader, v interface{}) {
	t.Helper()
	if err := json.NewDecoder(r).Decode(v); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
}
