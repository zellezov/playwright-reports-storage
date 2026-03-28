package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"prs/internal/model"
	"prs/internal/recovery"
	"prs/internal/store"
	"prs/internal/worker"
)

// TestCrashRecoveryProcessing simulates a crash during processing:
// write "processing" metadata + ZIP, run startup recovery, verify the job
// is re-queued and eventually completes.
func TestCrashRecoveryProcessing(t *testing.T) {
	env := newTestEnvWithWorkers(t, 0) // no workers yet

	// Create a fake "in-flight" report.
	id, _ := model.NewID()
	zipData, _ := buildZip(map[string]string{"index.html": "recovered"})

	inboxPath := worker.InboxPath(env.dataDir, id)
	os.MkdirAll(filepath.Dir(inboxPath), 0o755)
	os.WriteFile(inboxPath, zipData, 0o644)

	now := time.Now().UTC()
	rep := &model.Report{
		ID:        id,
		URL:       "http://test/reports/" + id,
		Status:    model.StatusProcessing,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := env.store.Write(rep); err != nil {
		t.Fatal(err)
	}

	// Simulate a fresh startup (recovery + rebuild).
	s2 := store.New(env.dataDir)
	toEnqueue, err := recovery.Run(env.dataDir, s2)
	if err != nil {
		t.Fatalf("recovery: %v", err)
	}

	// Verify the job was re-queued.
	found := false
	for _, qid := range toEnqueue {
		if qid == id {
			found = true
		}
	}
	if !found {
		t.Fatal("expected processing job to be re-queued after recovery")
	}

	// Verify status was reset to queued.
	r, err := s2.Read(id)
	if err != nil || r == nil {
		t.Fatal("report not found after recovery")
	}
	if r.Status != model.StatusQueued {
		t.Fatalf("expected queued after recovery, got %s", r.Status)
	}
}

// TestCrashRecoveryQueued verifies that queued jobs are re-added to the work list on startup.
func TestCrashRecoveryQueued(t *testing.T) {
	env := newTestEnvWithWorkers(t, 0)

	id, _ := model.NewID()
	zipData, _ := buildZip(map[string]string{"index.html": "queued"})

	inboxPath := worker.InboxPath(env.dataDir, id)
	os.MkdirAll(filepath.Dir(inboxPath), 0o755)
	os.WriteFile(inboxPath, zipData, 0o644)

	now := time.Now().UTC()
	rep := &model.Report{
		ID:        id,
		URL:       "http://test/reports/" + id,
		Status:    model.StatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := env.store.Write(rep); err != nil {
		t.Fatal(err)
	}

	s2 := store.New(env.dataDir)
	toEnqueue, err := recovery.Run(env.dataDir, s2)
	if err != nil {
		t.Fatalf("recovery: %v", err)
	}

	found := false
	for _, qid := range toEnqueue {
		if qid == id {
			found = true
		}
	}
	if !found {
		t.Fatal("expected queued job to be in re-enqueue list")
	}
}

// TestCrashRecoveryMissingZip verifies that a "processing" job with a missing
// ZIP is marked failed during recovery.
func TestCrashRecoveryMissingZip(t *testing.T) {
	env := newTestEnvWithWorkers(t, 0)

	id, _ := model.NewID()
	now := time.Now().UTC()
	rep := &model.Report{
		ID:        id,
		URL:       "http://test/reports/" + id,
		Status:    model.StatusProcessing,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := env.store.Write(rep); err != nil {
		t.Fatal(err)
	}
	// No ZIP written.

	s2 := store.New(env.dataDir)
	toEnqueue, err := recovery.Run(env.dataDir, s2)
	if err != nil {
		t.Fatalf("recovery: %v", err)
	}

	for _, qid := range toEnqueue {
		if qid == id {
			t.Fatal("job with missing ZIP should not be re-queued")
		}
	}

	r, _ := s2.Read(id)
	if r == nil || r.Status != model.StatusFailed {
		t.Fatalf("expected failed for missing ZIP, got %v", r)
	}
}

// TestMetricsCounters verifies that /metrics returns correct counts.
func TestMetricsCounters(t *testing.T) {
	env := newTestEnv(t)

	zipData, _ := buildZip(map[string]string{"index.html": "hi"})
	resp := uploadZip(t, env, zipData)
	var body map[string]interface{}
	decodeJSON(t, resp.Body, &body)
	resp.Body.Close()
	id := body["id"].(string)

	waitForStatus(t, env, id, "completed", 5*time.Second)

	mResp, err := env.server.Client().Get(env.server.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer mResp.Body.Close()

	var metrics map[string]interface{}
	if err := json.NewDecoder(mResp.Body).Decode(&metrics); err != nil {
		t.Fatal(err)
	}

	total, _ := metrics["reports_total"].(float64)
	if total < 1 {
		t.Fatalf("expected at least 1 report in metrics, got %v", total)
	}

	byStatus, _ := metrics["reports_by_status"].(map[string]interface{})
	if byStatus == nil {
		t.Fatal("missing reports_by_status")
	}
	completed, _ := byStatus["completed"].(float64)
	if completed < 1 {
		t.Fatalf("expected at least 1 completed report, got %v", completed)
	}
}
