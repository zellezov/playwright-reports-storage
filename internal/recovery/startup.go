package recovery

import (
	"archive/zip"
	"os"
	"time"

	"prs/internal/model"
	"prs/internal/store"
	"prs/internal/worker"
)

// Run performs the startup recovery sequence and returns IDs to re-enqueue.
// It must be called before workers start consuming the queue.
func Run(dataDir string, s *store.Store) ([]string, error) {
	reports, err := s.List()
	if err != nil {
		return nil, err
	}

	s.RebuildCounters(reports)

	var toEnqueue []string

	for _, r := range reports {
		switch r.Status {
		case model.StatusProcessing:
			toEnqueue = append(toEnqueue, recoverProcessing(dataDir, s, r)...)
		case model.StatusQueued:
			toEnqueue = append(toEnqueue, r.ID)
		}
	}

	return toEnqueue, nil
}

// recoverProcessing cleans up a partially-extracted report and re-queues or
// marks failed depending on whether the ZIP is still valid.
func recoverProcessing(dataDir string, s *store.Store, r *model.Report) []string {
	// Delete partial output directory.
	os.RemoveAll(worker.ReportDir(dataDir, r.ID))

	zipPath := worker.InboxPath(dataDir, r.ID)

	// Validate ZIP. We only care whether it opens without error — close immediately.
	zipReader, err := zip.OpenReader(zipPath)
	if err == nil {
		zipReader.Close()
		// ZIP is intact — reset to queued so the worker picks it up again.
		r.Status = model.StatusQueued
		r.UpdatedAt = time.Now().UTC()
		_ = s.Write(r)
		return []string{r.ID}
	}

	// ZIP missing or invalid — mark failed.
	r.Status = model.StatusFailed
	r.UpdatedAt = time.Now().UTC()
	_ = s.Write(r)
	return nil
}
