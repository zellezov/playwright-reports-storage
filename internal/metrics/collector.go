package metrics

import (
	"github.com/prometheus/client_golang/prometheus"

	"prs/internal/disk"
	"prs/internal/model"
	"prs/internal/store"
)

// PRSCollector implements prometheus.Collector. On each scrape it reads live
// data from the store and disk — no background syncing needed.
type PRSCollector struct {
	store   *store.Store
	dataDir string

	descReportsTotal    *prometheus.Desc
	descReportsByStatus *prometheus.Desc
	descDiskUsedBytes   *prometheus.Desc
	descDiskFreeBytes   *prometheus.Desc
}

// NewCollector creates a PRSCollector and registers it on reg.
func NewCollector(s *store.Store, dataDir string, reg prometheus.Registerer) *PRSCollector {
	c := &PRSCollector{
		store:   s,
		dataDir: dataDir,
		descReportsTotal: prometheus.NewDesc(
			"prs_reports_total",
			"Total number of reports currently on disk (all statuses).",
			nil, nil,
		),
		descReportsByStatus: prometheus.NewDesc(
			"prs_reports_by_status",
			"Number of reports grouped by status.",
			[]string{"status"}, nil,
		),
		descDiskUsedBytes: prometheus.NewDesc(
			"prs_disk_used_bytes",
			"Bytes used on the filesystem that holds the data directory.",
			nil, nil,
		),
		descDiskFreeBytes: prometheus.NewDesc(
			"prs_disk_free_bytes",
			"Bytes available on the filesystem that holds the data directory.",
			nil, nil,
		),
	}
	reg.MustRegister(c)
	return c
}

// Describe sends all metric descriptors to ch (required by prometheus.Collector).
func (c *PRSCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.descReportsTotal
	ch <- c.descReportsByStatus
	ch <- c.descDiskUsedBytes
	ch <- c.descDiskFreeBytes
}

// Collect reads current state and emits metrics. Called on every scrape.
func (c *PRSCollector) Collect(ch chan<- prometheus.Metric) {
	counters := c.store.Counters()

	total := 0
	for _, v := range counters {
		total += v
	}
	ch <- prometheus.MustNewConstMetric(c.descReportsTotal, prometheus.GaugeValue, float64(total))

	for _, status := range []model.Status{
		model.StatusQueued,
		model.StatusProcessing,
		model.StatusCompleted,
		model.StatusFailed,
	} {
		ch <- prometheus.MustNewConstMetric(
			c.descReportsByStatus,
			prometheus.GaugeValue,
			float64(counters[status]),
			string(status),
		)
	}

	diskUsed, diskFree := disk.Stats(c.dataDir)
	ch <- prometheus.MustNewConstMetric(c.descDiskUsedBytes, prometheus.GaugeValue, float64(diskUsed))
	ch <- prometheus.MustNewConstMetric(c.descDiskFreeBytes, prometheus.GaugeValue, float64(diskFree))
}
