package store

import (
	"testing"
	"time"

	"prs/internal/model"
)

// makeReport is a test helper that creates a Report with the given ID and status.
func makeReport(id string, status model.Status) *model.Report {
	now := time.Now().UTC()
	return &model.Report{
		ID:        id,
		URL:       "http://test/reports/" + id,
		Status:    status,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestWriteAndRead(t *testing.T) {
	s := New(t.TempDir())
	rep := makeReport("aabbccdd-0000-4000-8000-000000000001", model.StatusQueued)

	if err := s.Write(rep); err != nil {
		t.Fatal(err)
	}

	got, err := s.Read(rep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected report, got nil")
	}
	if got.ID != rep.ID {
		t.Errorf("ID: want %s, got %s", rep.ID, got.ID)
	}
	if got.Status != model.StatusQueued {
		t.Errorf("Status: want queued, got %s", got.Status)
	}
}

func TestReadMissingReturnsNil(t *testing.T) {
	s := New(t.TempDir())

	got, err := s.Read("aabbccdd-0000-4000-8000-000000000099")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil for missing report, got %+v", got)
	}
}

func TestDelete(t *testing.T) {
	s := New(t.TempDir())
	rep := makeReport("aabbccdd-0000-4000-8000-000000000002", model.StatusCompleted)
	s.Write(rep)

	if err := s.Delete(rep.ID); err != nil {
		t.Fatal(err)
	}

	got, _ := s.Read(rep.ID)
	if got != nil {
		t.Error("expected nil after delete, report still exists")
	}
}

func TestDeleteMissingIsNoOp(t *testing.T) {
	s := New(t.TempDir())
	// Deleting a non-existent report should not return an error.
	if err := s.Delete("aabbccdd-0000-4000-8000-000000000099"); err != nil {
		t.Errorf("unexpected error deleting missing report: %v", err)
	}
}

func TestCountersTrackStatusTransitions(t *testing.T) {
	s := New(t.TempDir())
	rep := makeReport("aabbccdd-0000-4000-8000-000000000003", model.StatusQueued)
	s.Write(rep)

	if c := s.Counters()[model.StatusQueued]; c != 1 {
		t.Errorf("after write: want 1 queued, got %d", c)
	}

	// Transition queued → processing.
	rep.Status = model.StatusProcessing
	s.Write(rep)

	counters := s.Counters()
	if counters[model.StatusQueued] != 0 {
		t.Errorf("after transition: want 0 queued, got %d", counters[model.StatusQueued])
	}
	if counters[model.StatusProcessing] != 1 {
		t.Errorf("after transition: want 1 processing, got %d", counters[model.StatusProcessing])
	}

	// Transition processing → completed.
	rep.Status = model.StatusCompleted
	s.Write(rep)

	counters = s.Counters()
	if counters[model.StatusProcessing] != 0 {
		t.Errorf("after completion: want 0 processing, got %d", counters[model.StatusProcessing])
	}
	if counters[model.StatusCompleted] != 1 {
		t.Errorf("after completion: want 1 completed, got %d", counters[model.StatusCompleted])
	}
}

func TestCountersDecrementOnDelete(t *testing.T) {
	s := New(t.TempDir())
	rep := makeReport("aabbccdd-0000-4000-8000-000000000004", model.StatusCompleted)
	s.Write(rep)
	s.Delete(rep.ID)

	if c := s.Counters()[model.StatusCompleted]; c != 0 {
		t.Errorf("after delete: want 0 completed, got %d", c)
	}
}

func TestList(t *testing.T) {
	s := New(t.TempDir())

	ids := []string{
		"aabbccdd-0000-4000-8000-000000000005",
		"aabbccdd-0000-4000-8000-000000000006",
		"11223344-0000-4000-8000-000000000007", // different shard
	}
	for _, id := range ids {
		s.Write(makeReport(id, model.StatusCompleted))
	}

	reports, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != len(ids) {
		t.Errorf("want %d reports, got %d", len(ids), len(reports))
	}
}

func TestRebuildCounters(t *testing.T) {
	s := New(t.TempDir())
	s.Write(makeReport("aabbccdd-0000-4000-8000-000000000008", model.StatusCompleted))
	s.Write(makeReport("aabbccdd-0000-4000-8000-000000000009", model.StatusFailed))
	s.Write(makeReport("aabbccdd-0000-4000-8000-000000000010", model.StatusQueued))

	// Simulate a restart: new store instance rebuilds counters from disk.
	s2 := New(s.dataDir)
	reports, _ := s2.List()
	s2.RebuildCounters(reports)

	counters := s2.Counters()
	if counters[model.StatusCompleted] != 1 {
		t.Errorf("completed: want 1, got %d", counters[model.StatusCompleted])
	}
	if counters[model.StatusFailed] != 1 {
		t.Errorf("failed: want 1, got %d", counters[model.StatusFailed])
	}
	if counters[model.StatusQueued] != 1 {
		t.Errorf("queued: want 1, got %d", counters[model.StatusQueued])
	}
}
