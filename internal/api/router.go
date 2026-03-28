package api

import (
	"net/http"
	"strings"
)

// NewRouter wires all routes and returns a http.Handler.
func NewRouter(h *Handler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", h.Health)
	mux.HandleFunc("/metrics", h.Metrics)
	mux.HandleFunc("/metrics/prometheus", h.PrometheusMetrics)

	mux.HandleFunc("/reports", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			h.ListReports(w, r)
		case http.MethodPost:
			h.UploadReport(w, r)
		case http.MethodDelete:
			h.DeleteAllReports(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// /reports/:id and sub-paths
	mux.HandleFunc("/reports/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Extract ID from path segment after /reports/
		rest := strings.TrimPrefix(path, "/reports/")
		parts := strings.SplitN(rest, "/", 2)
		id := parts[0]
		if id == "" {
			http.NotFound(w, r)
			return
		}

		suffix := ""
		if len(parts) > 1 {
			suffix = parts[1]
		}

		switch r.Method {
		case http.MethodGet:
			if suffix == "status" {
				h.GetReportStatus(w, r)
				return
			}
			h.GetReport(w, r)

		case http.MethodDelete:
			if suffix != "" {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			// Strip trailing slash from bare delete.
			if strings.HasSuffix(path, "/") && suffix == "" {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			_ = id
			h.DeleteReport(w, r)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	return mux
}
