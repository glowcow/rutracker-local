package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/glowcow/rutracker-local/internal/store"
)

// Request metrics; labels stay bounded: unknown methods fold to "other", the
// route is normalizeRoute's, the status a class.
var (
	httpRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "rutracker_http_requests_total",
		Help: "Total HTTP requests handled, by route and status class.",
	}, []string{"method", "route", "status_class"})

	httpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "rutracker_http_request_duration_seconds",
		Help: "HTTP request latency seconds, by route.",
		// Buckets tuned for our actual mix: search/forums are typically
		// <50 ms, full-table stats can spike to a second on a cold cache.
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	}, []string{"method", "route"})
)

// Content gauges — refreshed by a background goroutine. Cheap reads,
// constant labels, hardly any cardinality.
var (
	torrentsTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "rutracker_torrents_total",
		Help: "Current number of rows in torrents.",
	})

	forumsTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "rutracker_forums_total",
		Help: "Current number of distinct forums in torrents.",
	})

	torrentsSizeBytes = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "rutracker_torrents_size_bytes",
		Help: "Sum of size_bytes across all torrents.",
	})

	// Enough to see autovacuum fall behind without postgres_exporter; the
	// dashboard computes the ratio.
	torrentsLiveTuples = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "rutracker_torrents_live_tuples",
		Help: "n_live_tup from pg_stat_user_tables for the torrents table.",
	})

	torrentsDeadTuples = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "rutracker_torrents_dead_tuples",
		Help: "n_dead_tup from pg_stat_user_tables for the torrents table. Plot dead/(dead+live) and watch for sustained >20%.",
	})

	pgDatabaseSizeBytes = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "rutracker_pg_database_size_bytes",
		Help: "pg_database_size for the rutracker database (heap+indexes+TOAST+dead space).",
	})
)

// The latest run's gauges and the runs per status, read from parser_runs on
// the same cadence; the API never sees a run happen.
var (
	parserLastStart = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "rutracker_parser_last_run_started_timestamp_seconds",
		Help: "Unix start time of the most recent parser run.",
	})

	parserLastFinish = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "rutracker_parser_last_run_finished_timestamp_seconds",
		Help: "Unix finish time of the most recent parser run. 0 if still running.",
	})

	parserLastDuration = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "rutracker_parser_last_run_duration_seconds",
		Help: "Duration of the most recent finished parser run in seconds. 0 if still running.",
	})

	parserLastRowsInserted = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "rutracker_parser_last_run_rows_inserted",
		Help: "Rows inserted/updated by the most recent parser run.",
	})

	parserLastRowsSwept = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "rutracker_parser_last_run_rows_swept",
		Help: "Rows deleted by mark-and-sweep on the most recent parser run.",
	})

	// A run in progress shows as parserLastFinish 0, not as a status here.
	parserRunsTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "rutracker_parser_runs_total",
		Help: "Lifetime number of parser_runs rows by status.",
	}, []string{"status"})
)

func init() {
	prometheus.MustRegister(
		httpRequests, httpDuration,
		torrentsTotal, forumsTotal, torrentsSizeBytes,
		torrentsLiveTuples, torrentsDeadTuples, pgDatabaseSizeBytes,
		parserLastStart, parserLastFinish, parserLastDuration,
		parserLastRowsInserted, parserLastRowsSwept, parserRunsTotal,
	)
}

// normalizeRoute collapses paths to a bounded label set (torrent ids → {id},
// static assets → one) — else a crawler blows up metric cardinality.
func normalizeRoute(p string) string {
	switch {
	case p == "/healthz":
		return "/healthz"
	case p == "/metrics":
		return "/metrics"
	case p == "/api/stats":
		return "/api/stats"
	case p == "/api/forums":
		return "/api/forums"
	case p == "/api/search":
		return "/api/search"
	case strings.HasPrefix(p, "/api/torrents/"):
		return "/api/torrents/{id}"
	case strings.HasPrefix(p, "/api/favorites"):
		// Folds /api/favorites and /api/favorites/{id} onto one label —
		// the method already distinguishes list/add/remove/clear.
		return "/api/favorites"
	default:
		return "/spa"
	}
}

// normalizeMethod keeps the method label set bounded — see the label notes
// on httpRequests above.
func normalizeMethod(m string) string {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodOptions:
		return m
	default:
		return "other"
	}
}

// metrics counts and times every request, reading the status through the
// same statusRecorder as accessLog.
func metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		method := normalizeMethod(r.Method)
		route := normalizeRoute(r.URL.Path)
		statusClass := classifyStatus(rec.status)
		httpRequests.WithLabelValues(method, route, statusClass).Inc()
		httpDuration.WithLabelValues(method, route).Observe(time.Since(start).Seconds())
	})
}

func classifyStatus(code int) string {
	switch {
	case code >= 500:
		return "5xx"
	case code >= 400:
		return "4xx"
	case code >= 300:
		return "3xx"
	default:
		return "2xx"
	}
}

// startMetricsRefresher reads the gauges from Postgres at once and then every
// interval; an error is logged, the metrics are best effort.
func startMetricsRefresher(ctx context.Context, pool *pgxpool.Pool, interval time.Duration) {
	statsCache := &versionedCache[store.Stats]{}
	go func() {
		refreshMetrics(ctx, pool, statsCache)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				refreshMetrics(ctx, pool, statsCache)
			}
		}
	}()
}

func refreshMetrics(ctx context.Context, pool *pgxpool.Pool, statsCache *versionedCache[store.Stats]) {
	// Bound each query so a slow postgres can't stall the refresher into
	// the next tick.
	qctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// The content stats scan the whole table, so only a new succeeded run
	// recomputes them, or every tick would evict search's cache.
	version := latestParserVersion(qctx, pool)
	if stats, ok := statsCache.get(version); ok {
		setContentGauges(stats)
	} else if stats, err := store.GetStats(qctx, pool); err != nil {
		slog.Warn("metrics refresh: stats", "err", err)
	} else {
		statsCache.set(version, stats)
		setContentGauges(stats)
	}

	// The remaining queries are all cheap (pg_stat lookups + a tiny
	// parser_runs table) and run every tick.
	bloat, err := store.GetTableBloat(qctx, pool, "torrents")
	if err != nil {
		slog.Warn("metrics refresh: table bloat", "err", err)
	} else {
		torrentsLiveTuples.Set(float64(bloat.LiveTuples))
		torrentsDeadTuples.Set(float64(bloat.DeadTuples))
	}

	if dbSize, err := store.DatabaseSizeBytes(qctx, pool); err != nil {
		slog.Warn("metrics refresh: pg database size", "err", err)
	} else {
		pgDatabaseSizeBytes.Set(float64(dbSize))
	}

	counts, err := store.CountParserRuns(qctx, pool)
	if err != nil {
		slog.Warn("metrics refresh: parser run counts", "err", err)
	} else {
		parserRunsTotal.WithLabelValues(store.ParserRunSucceeded).Set(float64(counts.Succeeded))
		parserRunsTotal.WithLabelValues(store.ParserRunFailed).Set(float64(counts.Failed))
		parserRunsTotal.WithLabelValues(store.ParserRunRunning).Set(float64(counts.Running))
	}

	latest, err := store.LatestParserRun(qctx, pool)
	if err != nil {
		// "no runs yet" on a fresh deploy isn't a problem.
		if !errors.Is(err, store.ErrNotFound) {
			slog.Warn("metrics refresh: latest parser run", "err", err)
		}
		return
	}
	parserLastStart.Set(float64(latest.StartedAt.Unix()))
	parserLastRowsInserted.Set(float64(latest.RowsInserted))
	parserLastRowsSwept.Set(float64(latest.RowsSwept))
	if latest.FinishedAt != nil {
		parserLastFinish.Set(float64(latest.FinishedAt.Unix()))
		parserLastDuration.Set(latest.FinishedAt.Sub(latest.StartedAt).Seconds())
	} else {
		// Still running — leave finish/duration zeroed so the dashboard
		// can detect "in progress" without an extra label.
		parserLastFinish.Set(0)
		parserLastDuration.Set(0)
	}
}

func setContentGauges(stats store.Stats) {
	torrentsTotal.Set(float64(stats.TorrentsTotal))
	forumsTotal.Set(float64(stats.ForumsCount))
	torrentsSizeBytes.Set(float64(stats.TotalSizeBytes))
}
