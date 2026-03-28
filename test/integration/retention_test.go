package integration

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"prs/internal/cleanup"
	"prs/internal/model"
	"prs/internal/worker"
)

// writeExpiredReport writes a completed or failed report with an old CreatedAt.
func writeExpiredReport(t *testing.T, env *testEnv, status model.Status) string {
	t.Helper()

	id, err := model.NewID()
	if err != nil {
		t.Fatal(err)
	}

	past := time.Now().UTC().AddDate(0, 0, -10) // 10 days ago
	rep := &model.Report{
		ID:        id,
		URL:       "http://test/reports/" + id,
		Status:    status,
		CreatedAt: past,
		UpdatedAt: past,
	}
	if err := env.store.Write(rep); err != nil {
		t.Fatal(err)
	}

	// Create a dummy output directory for completed reports.
	if status == model.StatusCompleted {
		outDir := worker.ReportDir(env.dataDir, id)
		os.MkdirAll(outDir, 0o755)
		os.WriteFile(filepath.Join(outDir, "index.html"), []byte("hi"), 0o644)
	}

	return id
}

func TestRetentionDeletesExpired(t *testing.T) {
	env := newTestEnv(t)

	completedID := writeExpiredReport(t, env, model.StatusCompleted)
	failedID := writeExpiredReport(t, env, model.StatusFailed)

	cleaner := cleanup.New(env.dataDir, 5, 1*time.Millisecond, env.store)
	cleaner.Start()
	time.Sleep(100 * time.Millisecond)
	cleaner.Stop()

	// Both should be gone.
	for _, id := range []string{completedID, failedID} {
		r, _ := env.store.Read(id)
		if r != nil {
			t.Fatalf("expected report %s to be deleted by retention, still has status %s", id, r.Status)
		}
	}
}

func TestRetentionSkipsActive(t *testing.T) {
	env := newTestEnvWithWorkers(t, 0)

	// Write expired queued and processing reports.
	past := time.Now().UTC().AddDate(0, 0, -10)

	queuedID, _ := model.NewID()
	env.store.Write(&model.Report{
		ID: queuedID, URL: "http://test/reports/" + queuedID,
		Status: model.StatusQueued, CreatedAt: past, UpdatedAt: past,
	})

	processingID, _ := model.NewID()
	env.store.Write(&model.Report{
		ID: processingID, URL: "http://test/reports/" + processingID,
		Status: model.StatusProcessing, CreatedAt: past, UpdatedAt: past,
	})

	cleaner := cleanup.New(env.dataDir, 5, 1*time.Millisecond, env.store)
	cleaner.Start()
	time.Sleep(100 * time.Millisecond)
	cleaner.Stop()

	// Both should still exist.
	for _, id := range []string{queuedID, processingID} {
		r, _ := env.store.Read(id)
		if r == nil {
			t.Fatalf("expected active report %s to be retained", id)
		}
	}
}

func TestDeleteDuringProcessing(t *testing.T) {
	// Use a fake processing status to test the 409 path.
	env := newTestEnvWithWorkers(t, 0)

	id, _ := model.NewID()
	now := time.Now().UTC()
	env.store.Write(&model.Report{
		ID:        id,
		URL:       "http://test/reports/" + id,
		Status:    model.StatusProcessing,
		CreatedAt: now,
		UpdatedAt: now,
	})

	req, _ := http.NewRequest(http.MethodDelete, env.server.URL+"/reports/"+id, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for delete-during-processing, got %d", resp.StatusCode)
	}
}
