package api

import (
	"archive/zip"
	"encoding/json"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"prs/internal/disk"
	"prs/internal/metrics"
	"prs/internal/model"
	"prs/internal/staticfiles"
	"prs/internal/store"
	"prs/internal/worker"
)

const indexPageSize = 100

// Handler holds dependencies for all HTTP handlers.
type Handler struct {
	cfg          HandlerConfig
	store        *store.Store
	queue        *worker.Queue
	pool         *worker.Pool
	static       http.FileSystem
	failedTmpl   *template.Template
	indexTmpl    *template.Template
	promHandler  http.Handler
}

// HandlerConfig is the minimal config subset the handler needs.
type HandlerConfig struct {
	DataDir        string
	MaxUploadBytes int64
	BaseURL        string
	Workers        int
}

// failedPageData is passed to failed.html when rendering.
type failedPageData struct {
	Message string
}

// indexPageData is passed to index.html when rendering.
type indexPageData struct {
	Reports    []*model.Report
	Page       int
	TotalPages int
	Total      int
	From       int
	To         int
	HasPrev    bool
	HasNext    bool
	PrevPage   int
	NextPage   int
}

// New creates a Handler and parses embedded templates.
func New(cfg HandlerConfig, s *store.Store, q *worker.Queue, pool *worker.Pool, static http.FileSystem) *Handler {
	embeds := staticfiles.Files()

	failedTmpl := template.Must(template.ParseFS(embeds, "static/failed.html"))
	indexTmpl := template.Must(template.ParseFS(embeds, "static/index.html"))

	// Use a dedicated Prometheus registry so we don't pollute the global one.
	promRegistry := prometheus.NewRegistry()
	metrics.NewCollector(s, cfg.DataDir, promRegistry)

	return &Handler{
		cfg:         cfg,
		store:       s,
		queue:       q,
		pool:        pool,
		static:      static,
		failedTmpl:  failedTmpl,
		indexTmpl:   indexTmpl,
		promHandler: promhttp.HandlerFor(promRegistry, promhttp.HandlerOpts{}),
	}
}

// UploadReport handles POST /reports.
func (h *Handler) UploadReport(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form with 32 MB in-memory threshold.
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	defer r.MultipartForm.RemoveAll()

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Validate size.
	if header.Size > h.cfg.MaxUploadBytes {
		http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
		return
	}

	// Read file into memory/temp to allow ZIP validation.
	data, err := io.ReadAll(io.LimitReader(file, h.cfg.MaxUploadBytes+1))
	if err != nil {
		http.Error(w, "read error", http.StatusInternalServerError)
		return
	}
	if int64(len(data)) > h.cfg.MaxUploadBytes {
		http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
		return
	}

	// Validate ZIP by checking central directory.
	_, err = zip.NewReader(strings.NewReader(string(data)), int64(len(data)))
	if err != nil {
		http.Error(w, "not a valid ZIP file", http.StatusBadRequest)
		return
	}

	// Generate ID.
	id, err := model.NewID()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Save ZIP to inbox.
	inboxPath := worker.InboxPath(h.cfg.DataDir, id)
	if err := os.MkdirAll(filepath.Dir(inboxPath), 0o755); err != nil { // rwxr-xr-x: owner full, group+others read+execute
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(inboxPath, data, 0o644); err != nil { // rw-r--r--: owner read+write, group+others read-only
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	now := time.Now().UTC()
	rep := &model.Report{
		ID:        id,
		URL:       h.cfg.BaseURL + "/reports/" + id,
		Status:    model.StatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := h.store.Write(rep); err != nil {
		os.Remove(inboxPath)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.queue.Enqueue(id)

	slog.Info("upload received", "id", id, "size_bytes", len(data))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(rep)
}

// ListReports handles GET /reports.
// Returns JSON when Accept contains "application/json", otherwise an HTML page.
func (h *Handler) ListReports(w http.ResponseWriter, r *http.Request) {
	reports, err := h.store.List()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Sort newest first.
	sort.Slice(reports, func(i, j int) bool {
		return reports[i].CreatedAt.After(reports[j].CreatedAt)
	})

	// Offset-based pagination: ?page=N (1-indexed).
	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			page = n
		}
	}
	total := len(reports)
	start := min((page-1)*indexPageSize, total)
	end := min(start+indexPageSize, total)
	paginated := reports[start:end]

	// Content negotiation: JSON if explicitly requested, HTML otherwise.
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"reports":   paginated,
			"page":      page,
			"page_size": indexPageSize,
			"total":     total,
		})
		return
	}

	// Integer division truncates, so a remainder means there's a partial last page.
	// The second guard ensures we always show "page 1 of 1" even with zero reports.
	totalPages := total / indexPageSize
	if total%indexPageSize != 0 {
		totalPages++
	}
	if totalPages == 0 {
		totalPages = 1
	}

	pageData := indexPageData{
		Reports:    paginated,
		Page:       page,
		TotalPages: totalPages,
		Total:      total,
		From:       start + 1,
		To:         end,
		HasPrev:    page > 1,
		HasNext:    end < total,
		PrevPage:   page - 1,
		NextPage:   page + 1,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.indexTmpl.Execute(w, pageData)
}

// GetReport handles GET /reports/:id and GET /reports/:id/.
func (h *Handler) GetReport(w http.ResponseWriter, r *http.Request) {
	id := extractID(r.URL.Path, "/reports/")

	// Trailing-slash redirect.
	if !strings.HasSuffix(r.URL.Path, "/") {
		http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
		return
	}

	outDir := worker.ReportDir(h.cfg.DataDir, id)
	indexPath := filepath.Join(outDir, "index.html")

	if _, err := os.Stat(indexPath); err == nil {
		// Serve static files.
		stripped := http.StripPrefix("/reports/"+id, http.FileServer(http.Dir(outDir)))
		stripped.ServeHTTP(w, r)
		return
	}

	// Output not ready — check metadata.
	rep, err := h.store.Read(id)
	if err != nil || rep == nil {
		http.NotFound(w, r)
		return
	}

	switch rep.Status {
	case model.StatusQueued, model.StatusProcessing:
		h.serveStatic(w, "processing.html", http.StatusOK)
	case model.StatusCompleted:
		// Completed but index.html is missing — report the specific cause.
		h.serveFailedPage(w, "The report was processed successfully but index.html is missing from the archive.")
	case model.StatusFailed:
		h.serveFailedPage(w, "The report could not be processed. This may be due to insufficient disk space or a corrupt upload.")
	default:
		http.NotFound(w, r)
	}
}

// GetReportStatus handles GET /reports/:id/status.
func (h *Handler) GetReportStatus(w http.ResponseWriter, r *http.Request) {
	// Path: /reports/:id/status
	path := r.URL.Path
	path = strings.TrimSuffix(path, "/status")
	id := extractID(path, "/reports/")

	rep, err := h.store.Read(id)
	if err != nil || rep == nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rep)
}

// DeleteReport handles DELETE /reports/:id.
func (h *Handler) DeleteReport(w http.ResponseWriter, r *http.Request) {
	id := extractID(r.URL.Path, "/reports/")

	rep, err := h.store.Read(id)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if rep == nil {
		http.NotFound(w, r)
		return
	}

	if rep.Status == model.StatusProcessing {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"error": "report is currently being processed"})
		return
	}

	os.RemoveAll(worker.ReportDir(h.cfg.DataDir, id))
	os.Remove(worker.InboxPath(h.cfg.DataDir, id))
	_ = h.store.Delete(id)

	w.WriteHeader(http.StatusNoContent)
}

// DeleteAllReports handles DELETE /reports.
func (h *Handler) DeleteAllReports(w http.ResponseWriter, r *http.Request) {
	reports, err := h.store.List()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	for _, rep := range reports {
		if rep.Status == model.StatusProcessing {
			continue
		}
		os.RemoveAll(worker.ReportDir(h.cfg.DataDir, rep.ID))
		os.Remove(worker.InboxPath(h.cfg.DataDir, rep.ID))
		_ = h.store.Delete(rep.ID)
	}

	w.WriteHeader(http.StatusNoContent)
}

// Health handles GET /health.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	counters := h.store.Counters()
	queueDepth := counters[model.StatusQueued] + counters[model.StatusProcessing]

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":      "ok",
		"queue_depth": queueDepth,
		"workers":     h.cfg.Workers,
	})
}

// Metrics handles GET /metrics — returns a JSON summary for quick inspection.
func (h *Handler) Metrics(w http.ResponseWriter, r *http.Request) {
	counters := h.store.Counters()

	total := 0
	for _, v := range counters {
		total += v
	}

	countsByStatus := map[string]int{
		"queued":     counters[model.StatusQueued],
		"processing": counters[model.StatusProcessing],
		"completed":  counters[model.StatusCompleted],
		"failed":     counters[model.StatusFailed],
	}

	diskUsed, diskFree := disk.Stats(h.cfg.DataDir)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"reports_total":     total,
		"reports_by_status": countsByStatus,
		"disk_used_bytes":   diskUsed,
		"disk_free_bytes":   diskFree,
	})
}

// PrometheusMetrics handles GET /metrics/prometheus — standard Prometheus scrape endpoint.
func (h *Handler) PrometheusMetrics(w http.ResponseWriter, r *http.Request) {
	h.promHandler.ServeHTTP(w, r)
}

// serveStatic serves a named file from the embedded static filesystem.
func (h *Handler) serveStatic(w http.ResponseWriter, name string, code int) {
	f, err := h.static.Open(name)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	io.Copy(w, f)
}

// serveFailedPage renders failed.html with the given error message.
func (h *Handler) serveFailedPage(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.failedTmpl.Execute(w, failedPageData{Message: message})
}

// extractID strips prefix from path and returns the first path segment after it.
func extractID(path, prefix string) string {
	s := strings.TrimPrefix(path, prefix)
	// Remove any trailing slash or sub-path.
	if idx := strings.Index(s, "/"); idx != -1 {
		s = s[:idx]
	}
	return s
}
