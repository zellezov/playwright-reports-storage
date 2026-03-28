package recovery

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"prs/internal/model"
	"prs/internal/store"
	"prs/internal/worker"
)

// buildZip creates a minimal valid in-memory ZIP with a single index.html.
func buildZip() []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, _ := zw.Create("index.html")
	f.Write([]byte("test"))
	zw.Close()
	return buf.Bytes()
}

func writeMetadata(t *testing.T, s *store.Store, id string, status model.Status) {
	t.Helper()
	now := time.Now().UTC()
	if err := s.Write(&model.Report{
		ID: id, URL: "http://test/reports/" + id,
		Status: status, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
}

func writeZip(t *testing.T, dataDir, id string) {
	t.Helper()
	inboxPath := worker.InboxPath(dataDir, id)
	os.MkdirAll(filepath.Dir(inboxPath), 0o755) // rwxr-xr-x: owner full, group+others read+execute
	if err := os.WriteFile(inboxPath, buildZip(), 0o644); err != nil { // rw-r--r--: owner read+write, group+others read-only
		t.Fatal(err)
	}
}

func TestRecoverProcessingWithValidZip(t *testing.T) {
	dataDir := t.TempDir()
	s := store.New(dataDir)

	id, _ := model.NewID()
	writeMetadata(t, s, id, model.StatusProcessing)
	writeZip(t, dataDir, id)

	toEnqueue, err := Run(dataDir, s)
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Contains(toEnqueue, id) {
		t.Error("processing job with valid ZIP should be re-queued")
	}

	r, _ := s.Read(id)
	if r.Status != model.StatusQueued {
		t.Errorf("want queued after recovery, got %s", r.Status)
	}
}

func TestRecoverProcessingWithMissingZip(t *testing.T) {
	dataDir := t.TempDir()
	s := store.New(dataDir)

	id, _ := model.NewID()
	writeMetadata(t, s, id, model.StatusProcessing)
	// No ZIP written — simulates a crash where the file was never saved.

	toEnqueue, err := Run(dataDir, s)
	if err != nil {
		t.Fatal(err)
	}

	if slices.Contains(toEnqueue, id) {
		t.Error("job with missing ZIP should not be re-queued")
	}

	r, _ := s.Read(id)
	if r.Status != model.StatusFailed {
		t.Errorf("want failed for missing ZIP, got %s", r.Status)
	}
}

func TestRecoverQueuedJobIsEnqueued(t *testing.T) {
	dataDir := t.TempDir()
	s := store.New(dataDir)

	id, _ := model.NewID()
	writeMetadata(t, s, id, model.StatusQueued)

	toEnqueue, err := Run(dataDir, s)
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Contains(toEnqueue, id) {
		t.Error("queued job should be returned for re-enqueue")
	}
}

func TestRecoverCompletedAndFailedAreUntouched(t *testing.T) {
	dataDir := t.TempDir()
	s := store.New(dataDir)

	completedID, _ := model.NewID()
	failedID, _ := model.NewID()
	writeMetadata(t, s, completedID, model.StatusCompleted)
	writeMetadata(t, s, failedID, model.StatusFailed)

	toEnqueue, _ := Run(dataDir, s)

	if slices.Contains(toEnqueue, completedID) {
		t.Error("completed job should not be re-queued")
	}
	if slices.Contains(toEnqueue, failedID) {
		t.Error("failed job should not be re-queued")
	}

	// Statuses should be unchanged.
	r, _ := s.Read(completedID)
	if r.Status != model.StatusCompleted {
		t.Errorf("completed status should be unchanged, got %s", r.Status)
	}
}

