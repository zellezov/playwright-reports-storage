package cleanup

import (
	"testing"
	"time"

	"prs/internal/model"
	"prs/internal/store"
)

func makeReport(id string, status model.Status, age time.Duration) *model.Report {
	ts := time.Now().UTC().Add(-age)
	return &model.Report{
		ID:        id,
		URL:       "http://test/reports/" + id,
		Status:    status,
		CreatedAt: ts,
		UpdatedAt: ts,
	}
}

func TestCleanDeletesExpiredReports(t *testing.T) {
	dataDir := t.TempDir()
	s := store.New(dataDir)

	expiredID := "aabbccdd-0000-4000-8000-000000000020"
	freshID := "aabbccdd-0000-4000-8000-000000000021"

	s.Write(makeReport(expiredID, model.StatusCompleted, 10*24*time.Hour)) // 10 days old, past 5-day cutoff
	s.Write(makeReport(freshID, model.StatusCompleted, 1*time.Hour))        // 1 hour old, within cutoff

	r := New(dataDir, 5, time.Hour, s)
	r.clean()

	if rep, _ := s.Read(expiredID); rep != nil {
		t.Errorf("expired report should have been deleted, still has status %s", rep.Status)
	}
	if rep, _ := s.Read(freshID); rep == nil {
		t.Error("fresh report should not have been deleted")
	}
}

func TestCleanSkipsQueuedAndProcessing(t *testing.T) {
	dataDir := t.TempDir()
	s := store.New(dataDir)

	queuedID := "aabbccdd-0000-4000-8000-000000000022"
	processingID := "aabbccdd-0000-4000-8000-000000000023"

	// Both are expired by age but must be skipped because they're active.
	s.Write(makeReport(queuedID, model.StatusQueued, 10*24*time.Hour))
	s.Write(makeReport(processingID, model.StatusProcessing, 10*24*time.Hour))

	r := New(dataDir, 5, time.Hour, s)
	r.clean()

	if rep, _ := s.Read(queuedID); rep == nil {
		t.Error("queued report should not be deleted even when expired")
	}
	if rep, _ := s.Read(processingID); rep == nil {
		t.Error("processing report should not be deleted even when expired")
	}
}

func TestCleanDeletesExpiredFailed(t *testing.T) {
	dataDir := t.TempDir()
	s := store.New(dataDir)

	id := "aabbccdd-0000-4000-8000-000000000024"
	s.Write(makeReport(id, model.StatusFailed, 10*24*time.Hour))

	r := New(dataDir, 5, time.Hour, s)
	r.clean()

	if rep, _ := s.Read(id); rep != nil {
		t.Error("expired failed report should have been deleted")
	}
}
