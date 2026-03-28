# Playwright Report Storage (PRS)

A lightweight, self-hosted file server for storing and serving Playwright HTML reports. It decouples report artifacts from CI infrastructure — instead of bloating Jenkins/GitLab/GitHub Actions with large ZIP attachments, CI uploads once and gets back a stable URL immediately.

```
CI pipeline → POST /reports (ZIP) → PRS returns URL → CI embeds URL in build result
                                                      ↓
                                          User opens URL in browser
```

## Features

- Accepts uploads and returns a URL instantly; extraction happens asynchronously
- FIFO worker pool for background ZIP extraction
- Browse all uploaded reports at `GET /reports`
- Auto-recovers interrupted jobs on restart (no manual intervention after a crash)
- Configurable retention — expired reports are cleaned up automatically
- Prometheus-compatible metrics endpoint alongside a JSON summary endpoint
- Structured logging with configurable level and format
- Single self-contained binary, Linux only

## Requirements

- Go 1.23+
- Linux — disk space checks use `unix.Statfs` which is Linux/macOS only; Windows is not supported

## Installation

```bash
git clone <repo>
cd playwright-reports-storage
make build        # produces ./prs binary
```

Or without Make:

```bash
go build -o prs ./cmd/prs
```

## Running

```bash
./prs
# Listening on :3912, data stored in ./data/
```

Override any default via environment variable:

```bash
PRS_PORT=8080 PRS_BASE_URL=https://prs.example.com ./prs
```

## Configuration

| Variable | Default | Description |
|---|---|---|
| `PRS_PORT` | `3912` | HTTP listen port |
| `PRS_DATA_DIR` | `./data` | Root directory for all stored data |
| `PRS_BASE_URL` | `http://localhost:3912` | Public base URL embedded in returned report URLs — **set this in production** |
| `PRS_MAX_UPLOAD_BYTES` | `2684354560` | Maximum accepted upload size (2.5 GB) |
| `PRS_WORKERS` | `2` | Number of parallel extraction workers |
| `PRS_RETENTION_DAYS` | `5` | Reports older than this are deleted automatically |
| `PRS_CLEANUP_INTERVAL` | `1h` | How often the retention sweep runs |
| `PRS_DISK_EXPANSION_FACTOR` | `1.5` | Before extracting, checks that free disk ≥ zip_size × factor |
| `PRS_LOG_LEVEL` | `info` | Log verbosity: `debug`, `info`, or `error` |
| `PRS_LOG_FORMAT` | `text` | Log output format: `text` (human-readable) or `json` (for log aggregators) |

## API

### List reports
```
GET /reports
```
Returns an HTML page listing all reports, newest first, paginated at 100 per page (`?page=N`).
Add `Accept: application/json` to get a JSON response instead:
```json
{ "reports": [...], "page": 1, "page_size": 100, "total": 42 }
```

### Upload a report
```
POST /reports
Content-Type: multipart/form-data
field: file=<zip>
```
Returns `201` with `{ id, url, status, created_at }`.

> **ZIP structure:** `index.html` must be at the root of the archive, not inside a subdirectory.
> Playwright's HTML reporter writes output to `playwright-report/` — zip from *inside* that directory:
> ```bash
> cd playwright-report && zip -r ../report.zip . && cd ..
> ```

### View a report
```
GET /reports/:id/        (trailing slash required; bare URL redirects automatically)
```
- If still processing: returns an auto-refreshing page (polls every 5 s)
- If failed: returns an error page with a specific message
- If ready: serves the full Playwright report as static files

### Check status
```
GET /reports/:id/status  → { id, url, status, created_at, updated_at }
```
Status values: `queued` | `processing` | `completed` | `failed`

### Delete
```
DELETE /reports/:id      → 204, or 409 if currently processing
DELETE /reports          → 204 (skips reports currently being processed)
```

### Health
```
GET /health  → { status, queue_depth, workers }
```

### Metrics
```
GET /metrics             → JSON summary for quick inspection
GET /metrics/prometheus  → Prometheus text format for scraping
```

JSON response:
```json
{
  "reports_total": 142,
  "reports_by_status": { "queued": 1, "processing": 0, "completed": 140, "failed": 1 },
  "disk_used_bytes": 21474836480,
  "disk_free_bytes": 107374182400
}
```

Prometheus metrics: `prs_reports_total`, `prs_reports_by_status{status=...}`, `prs_disk_used_bytes`, `prs_disk_free_bytes`.

## Uploading from CI

```bash
# Generic example (curl)
REPORT_URL=$(curl -sf -F "file=@playwright-report.zip" https://prs.example.com/reports \
  | jq -r .url)
echo "Report: $REPORT_URL"
```

The server responds immediately — you do not need to poll for completion before embedding the URL in build results. The report page handles the "still processing" state itself.

## Edge Cases

**Crash during extraction** — if the process is killed mid-extraction, the partially written output directory is cleaned up on the next startup and the job is re-queued automatically. No manual intervention is needed.

**Disk full** — the free-space check happens in the worker just before extraction, not at upload time. If there is not enough room, the report is marked `failed` and the ZIP is deleted. The upload itself succeeds with a `queued` status; the failure surfaces via `GET /reports/:id/status` or the report page.

**Corrupt or truncated ZIP** — validated at upload time (central directory check). Invalid ZIPs are rejected with `400` and nothing is written to disk. A ZIP that passes upload validation but turns out unreadable during extraction results in `failed` status.

**Reports in flight during shutdown** — on `SIGTERM` the server stops accepting new uploads and waits up to 30 seconds for active workers to finish their current job before exiting. Any job still processing at force-exit is recovered on next start.

**Retention skips active jobs** — the retention sweep never deletes a report in `queued` or `processing` state, even if it is past the cutoff date.

**No auth in v1** — the upload and delete endpoints are unauthenticated. Restrict access at the network level (firewall, reverse-proxy `allow` rules) until token auth is added.

## Development

```bash
make test               # all tests (unit + integration)
make test-integration   # integration tests only, with verbose output
make coverage           # unit + integration coverage report
make lint               # go vet
make build              # compile binary to ./prs
make clean              # remove binary and coverage output
```

## License

MIT
