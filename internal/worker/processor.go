package worker

import (
	"archive/zip"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"prs/internal/disk"
	"prs/internal/model"
	"prs/internal/store"
)

// Processor extracts ZIP reports to the output directory.
type Processor struct {
	dataDir             string
	diskExpansionFactor float64
	store               *store.Store
}

// NewProcessor creates a Processor.
func NewProcessor(dataDir string, expansionFactor float64, s *store.Store) *Processor {
	return &Processor{
		dataDir:             dataDir,
		diskExpansionFactor: expansionFactor,
		store:               s,
	}
}

// Process handles a single report job: disk check → extract → update status.
func (p *Processor) Process(id string) {
	r, err := p.store.Read(id)
	if err != nil || r == nil {
		return
	}

	zipPath := InboxPath(p.dataDir, id)

	// Disk space pre-check.
	info, err := os.Stat(zipPath)
	if err != nil {
		slog.Error("failed to stat ZIP", "id", id, "err", err)
		p.markFailed(r, zipPath)
		return
	}
	required := int64(float64(info.Size()) * p.diskExpansionFactor)
	ok, err := disk.HasSpace(p.dataDir, required)
	if err != nil {
		slog.Error("disk space check failed", "id", id, "err", err)
		p.markFailed(r, zipPath)
		return
	}
	if !ok {
		slog.Error("insufficient disk space", "id", id, "required_bytes", required)
		p.markFailed(r, zipPath)
		return
	}

	// Mark processing.
	r.Status = model.StatusProcessing
	r.UpdatedAt = time.Now().UTC()
	if err := p.store.Write(r); err != nil {
		return
	}

	slog.Debug("extracting report", "id", id, "size_bytes", info.Size())
	outDir := ReportDir(p.dataDir, id)
	if err := p.extractZip(zipPath, outDir); err != nil {
		slog.Error("extraction failed", "id", id, "err", err)
		_ = os.RemoveAll(outDir)
		p.markFailed(r, zipPath)
		return
	}

	// Success.
	os.Remove(zipPath)
	r.Status = model.StatusCompleted
	r.UpdatedAt = time.Now().UTC()
	_ = p.store.Write(r)
	slog.Debug("report extraction completed", "id", id)
}

func (p *Processor) markFailed(r *model.Report, zipPath string) {
	os.Remove(zipPath)
	r.Status = model.StatusFailed
	r.UpdatedAt = time.Now().UTC()
	_ = p.store.Write(r)
}

func (p *Processor) extractZip(zipPath, outDir string) error {
	zipReader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer zipReader.Close()

	if err := os.MkdirAll(outDir, 0o755); err != nil { // rwxr-xr-x: owner full, group+others read+execute
		return err
	}

	for _, zipEntry := range zipReader.File {
		if err := extractFile(zipEntry, outDir); err != nil {
			return err
		}
	}
	return nil
}

func extractFile(f *zip.File, outDir string) error {
	// Security: prevent path traversal.
	rel := filepath.Clean(f.Name)
	if strings.HasPrefix(rel, "..") {
		return fmt.Errorf("zip entry %q escapes destination", f.Name)
	}
	dest := filepath.Join(outDir, rel)

	if f.FileInfo().IsDir() {
		return os.MkdirAll(dest, 0o755) // rwxr-xr-x: owner full, group+others read+execute
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil { // rwxr-xr-x: owner full, group+others read+execute
		return err
	}

	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, rc)
	return err
}

// InboxPath returns the path to the inbox ZIP for id.
func InboxPath(dataDir, id string) string {
	shard := id[:2]
	return filepath.Join(dataDir, "inbox", shard, id+".zip")
}

// ReportDir returns the output directory for id.
func ReportDir(dataDir, id string) string {
	shard := id[:2]
	return filepath.Join(dataDir, "reports", shard, id)
}
