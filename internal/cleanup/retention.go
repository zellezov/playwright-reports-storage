package cleanup

import (
	"log/slog"
	"os"
	"time"

	"prs/internal/model"
	"prs/internal/store"
	"prs/internal/worker"
)

// Retention runs periodic cleanup of expired reports.
type Retention struct {
	dataDir       string
	retentionDays int
	interval      time.Duration
	store         *store.Store
	stop          chan struct{}
}

// New creates a Retention cleaner.
func New(dataDir string, retentionDays int, interval time.Duration, s *store.Store) *Retention {
	return &Retention{
		dataDir:       dataDir,
		retentionDays: retentionDays,
		interval:      interval,
		store:         s,
		stop:          make(chan struct{}),
	}
}

// Start launches the background cleanup goroutine.
func (r *Retention) Start() {
	go r.run()
}

// Stop signals the cleanup goroutine to exit.
func (r *Retention) Stop() {
	close(r.stop)
}

func (r *Retention) run() {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			r.clean()
		case <-r.stop:
			return
		}
	}
}

func (r *Retention) clean() {
	cutoff := time.Now().UTC().AddDate(0, 0, -r.retentionDays)
	slog.Info("cleanup: starting", "cutoff", cutoff.Format(time.RFC3339))

	reports, err := r.store.List()
	if err != nil {
		slog.Error("cleanup: failed to list reports", "err", err)
		return
	}

	deleted, skipped := 0, 0
	for _, rep := range reports {
		// Active jobs are never deleted, even if past the retention cutoff.
		if rep.Status == model.StatusProcessing || rep.Status == model.StatusQueued {
			skipped++
			continue
		}
		if rep.CreatedAt.After(cutoff) {
			continue
		}
		r.deleteReport(rep.ID)
		deleted++
	}

	slog.Info("cleanup: finished", "deleted", deleted, "skipped_active", skipped)
}

func (r *Retention) deleteReport(id string) {
	os.RemoveAll(worker.ReportDir(r.dataDir, id))
	os.Remove(worker.InboxPath(r.dataDir, id))
	_ = r.store.Delete(id)
}
