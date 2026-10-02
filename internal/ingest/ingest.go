// Package ingest runs a dump parse: the whole pipeline (decode → batch →
// COPY-upsert → optional sweep) plus parser_runs bookkeeping. Single entry
// point for the CLI (`rutracker parse`) and the in-app admin endpoint. An
// optional Emitter streams progress/log/status events (CLI nil; server → SSE).
package ingest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"github.com/glowcow/rutracker-local/internal/parser"
	"github.com/glowcow/rutracker-local/internal/store"
)

// Options configures one run. Zero NumWorkers/BatchSize are clamped to sane
// defaults, so a partly-filled struct (e.g. from a JSON request body) is safe.
type Options struct {
	Source     string
	BatchSize  int
	NumWorkers int
	Sweep      bool
}

// Result is the outcome of a successful run.
type Result struct {
	RunID    int64
	Rows     int64
	Swept    int64
	Duration time.Duration
}

// Progress is a point-in-time snapshot, emitted every few seconds. Percent and
// ETA are byte-based (the parser doesn't know the total row count up front);
// Rows is the running count with no denominator.
type Progress struct {
	Pct       float64 `json:"pct"`
	Rows      int64   `json:"rows"`
	ReadMB    int64   `json:"read_mb"`
	TotalMB   int64   `json:"total_mb"`
	RateRowsS int64   `json:"rate_rows_s"`
	EtaS      int64   `json:"eta_s"`
	ElapsedS  int64   `json:"elapsed_s"`
}

// Event is one item on the live stream. Kind selects which fields are set.
type Event struct {
	Kind     string    `json:"kind"`               // "status" | "log" | "progress"
	TS       string    `json:"ts"`                 // RFC3339, stamped on emit
	Status   string    `json:"status,omitempty"`   // kind=status: running|succeeded|failed
	RunID    int64     `json:"run_id,omitempty"`   // kind=status
	Level    string    `json:"level,omitempty"`    // kind=log: info|warn|error
	Msg      string    `json:"msg,omitempty"`      // kind=log
	Detail   string    `json:"detail,omitempty"`   // kind=log
	Progress *Progress `json:"progress,omitempty"` // kind=progress
}

// Emitter receives events. It may be called from multiple goroutines (the
// progress ticker runs alongside the workers), so implementations must be safe
// for concurrent use. A nil Emitter is a no-op.
type Emitter func(Event)

func (e Emitter) send(ev Event) {
	if e == nil {
		return
	}
	ev.TS = time.Now().UTC().Format(time.RFC3339)
	e(ev)
}

func (e Emitter) log(level, msg, detail string) {
	e.send(Event{Kind: "log", Level: level, Msg: msg, Detail: detail})
}

// countingReader wraps an io.Reader and tracks total bytes read atomically, so
// the progress goroutine can sample without coordination.
type countingReader struct {
	r io.Reader
	n atomic.Int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n.Add(int64(n))
	return n, err
}

// Run streams the dump at opts.Source into Postgres and (optionally) sweeps
// rows the dump no longer contains. It opens its own pgxpool sized to the
// worker count so parse connections never contend with the server's request
// pool. Blocks until the run finishes; honours ctx cancellation.
func Run(ctx context.Context, databaseURL string, opts Options, emit Emitter) (Result, error) {
	if opts.Source == "" {
		return Result{}, errors.New("source is required")
	}
	if opts.NumWorkers < 1 {
		opts.NumWorkers = 4
	}
	// A non-positive batch size would panic in make() inside the decoder
	// goroutine — after StartParserRun already inserted its row, orphaning a
	// `running` entry.
	if opts.BatchSize < 1 {
		opts.BatchSize = 500
	}

	// Own pool: parse needs N worker connections + 1 spare for setup queries.
	pgCfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return Result{}, fmt.Errorf("parse pg url: %w", err)
	}
	pgCfg.MaxConns = int32(opts.NumWorkers + 1)
	pool, err := pgxpool.NewWithConfig(ctx, pgCfg)
	if err != nil {
		return Result{}, fmt.Errorf("pgx pool: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return Result{}, fmt.Errorf("ping postgres: %w", err)
	}

	resolved, err := parser.ResolveSource(opts.Source)
	if err != nil {
		return Result{}, err
	}
	if resolved != opts.Source {
		slog.Info("glob resolved", "pattern", opts.Source, "file", resolved)
		emit.log("info", "glob resolved", opts.Source+" → "+resolved)
	}

	f, err := os.Open(resolved)
	if err != nil {
		return Result{}, fmt.Errorf("open %s: %w", resolved, err)
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return Result{}, fmt.Errorf("stat %s: %w", resolved, err)
	}
	totalBytes := st.Size()

	counter := &countingReader{r: f}
	src, err := parser.DecompressorFor(resolved, counter)
	if err != nil {
		return Result{}, err
	}
	defer src.Close()

	// Pull the sweep cutoff from postgres BEFORE any ingest query fires. Using
	// the db clock (not Go's) keeps the "untouched before this run" comparison
	// safe across host/db clock skew — every NOW() the parse touches after this
	// is guaranteed ≥ sweepCutoff.
	var sweepCutoff time.Time
	if opts.Sweep {
		sweepCutoff, err = store.ParseStartNow(ctx, pool)
		if err != nil {
			return Result{}, err
		}
	}

	// Durable record, inserted `running` up front so an interrupted parse
	// leaves a clear marker (surfaced by /metrics and the admin panel).
	runID, err := store.StartParserRun(ctx, pool, resolved, opts.Sweep)
	if err != nil {
		return Result{}, err
	}

	slog.Info("parse start",
		"source", resolved,
		"size_mb", totalBytes/(1<<20),
		"batch_size", opts.BatchSize,
		"workers", opts.NumWorkers,
		"sweep", opts.Sweep,
		"run_id", runID,
	)
	emit.send(Event{Kind: "status", Status: store.ParserRunRunning, RunID: runID})
	emit.log("info", "parse start", fmt.Sprintf("%s  %d MB  batch=%d workers=%d sweep=%v  run#%d",
		resolved, totalBytes/(1<<20), opts.BatchSize, opts.NumWorkers, opts.Sweep, runID))

	start := time.Now()
	var total atomic.Int64

	// Pipeline: 1 decoder pushes batches into a buffered channel; N workers each
	// own a CopyIngester (one pgx connection + session-local TEMP table) and
	// consume from the same channel. errgroup propagates the first error.
	g, gctx := errgroup.WithContext(ctx)
	batches := make(chan []store.Torrent, opts.NumWorkers)

	g.Go(func() error {
		defer close(batches)
		batch := make([]store.Torrent, 0, opts.BatchSize)
		err := parser.Parse(src, func(t store.Torrent) error {
			batch = append(batch, t)
			if len(batch) < opts.BatchSize {
				return nil
			}
			select {
			case batches <- batch:
			case <-gctx.Done():
				return gctx.Err()
			}
			batch = make([]store.Torrent, 0, opts.BatchSize)
			return nil
		})
		if err != nil {
			return err
		}
		if len(batch) > 0 {
			select {
			case batches <- batch:
			case <-gctx.Done():
				return gctx.Err()
			}
		}
		return nil
	})

	for i := 0; i < opts.NumWorkers; i++ {
		g.Go(func() error {
			ing, err := store.NewCopyIngester(gctx, pool)
			if err != nil {
				return err
			}
			defer ing.Close()
			for b := range batches {
				n, err := ing.Ingest(gctx, b)
				if err != nil {
					return err
				}
				total.Add(int64(n))
			}
			return nil
		})
	}

	// Progress logger — runs alongside g, killed via progressCtx once g.Wait
	// returns. Emits both a slog line and a stream event every tick.
	progressCtx, stopProgress := context.WithCancel(ctx)
	progressDone := make(chan struct{})
	go func() {
		defer close(progressDone)
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-progressCtx.Done():
				return
			case now := <-t.C:
				p := snapshot(start, now, total.Load(), counter.n.Load(), totalBytes)
				slog.Info("parse progress",
					"rows", p.Rows, "read_mb", p.ReadMB, "total_mb", p.TotalMB,
					"pct", fmt.Sprintf("%.1f", p.Pct), "rate_rows_s", p.RateRowsS, "eta_s", p.EtaS)
				emit.send(Event{Kind: "progress", Progress: &p})
				emit.log("info", "parse progress", fmt.Sprintf("rows=%d  %d/%d MB  %.1f%%  %d/s  eta %s",
					p.Rows, p.ReadMB, p.TotalMB, p.Pct, p.RateRowsS, fmtDur(p.EtaS)))
			}
		}
	}()

	waitErr := g.Wait()
	stopProgress()
	<-progressDone

	// Reap the decompressor and fold its exit status into the verdict BEFORE
	// deciding success — a truncated .xz that ends on a clean XML boundary
	// leaves the exit code as the only failure signal, and the next step on the
	// success path is the sweep (deleting rows).
	if closeErr := src.Close(); closeErr != nil && waitErr == nil {
		waitErr = closeErr
	}

	if waitErr != nil {
		if fErr := store.FailParserRun(context.WithoutCancel(ctx), pool, runID); fErr != nil {
			slog.Warn("parser_runs: failed to record FAILED", "err", fErr, "run_id", runID)
		}
		emit.log("error", "parse failed", waitErr.Error())
		emit.send(Event{Kind: "status", Status: store.ParserRunFailed, RunID: runID})
		return Result{RunID: runID}, waitErr
	}

	dur := time.Since(start)
	rows := total.Load()
	rate := int64(0)
	if dur.Seconds() > 0 {
		rate = int64(float64(rows) / dur.Seconds())
	}
	slog.Info("parse done", "rows", rows, "duration_s", int64(dur.Seconds()), "rate_rows_s", rate)
	emit.log("info", "parse done", fmt.Sprintf("rows=%d  %s  %d/s", rows, fmtDur(int64(dur.Seconds())), rate))
	// Final 100% snapshot so the panel shows completed stats (rows/rate/bar)
	// even when the parse finished before the 5 s progress ticker ever fired —
	// which is every small dump.
	mb := totalBytes / (1 << 20)
	emit.send(Event{Kind: "progress", Progress: &Progress{
		Pct: 100, Rows: rows, ReadMB: mb, TotalMB: mb,
		RateRowsS: rate, EtaS: 0, ElapsedS: int64(dur.Seconds()),
	}})

	var swept int64
	if opts.Sweep {
		swept, err = store.Sweep(ctx, pool, sweepCutoff)
		if err != nil {
			if fErr := store.FailParserRun(context.WithoutCancel(ctx), pool, runID); fErr != nil {
				slog.Warn("parser_runs: failed to record FAILED", "err", fErr, "run_id", runID)
			}
			emit.log("error", "sweep failed", err.Error())
			emit.send(Event{Kind: "status", Status: store.ParserRunFailed, RunID: runID})
			return Result{RunID: runID}, err
		}
		slog.Info("sweep done", "deleted", swept, "cutoff", sweepCutoff.UTC().Format(time.RFC3339))
		emit.log("info", "sweep done", fmt.Sprintf("deleted=%d", swept))
	}

	if err := store.FinishParserRun(ctx, pool, runID, rows, swept); err != nil {
		slog.Warn("parser_runs: failed to record SUCCEEDED", "err", err, "run_id", runID)
	}
	emit.send(Event{Kind: "status", Status: store.ParserRunSucceeded, RunID: runID})
	return Result{RunID: runID, Rows: rows, Swept: swept, Duration: dur}, nil
}

// snapshot computes a progress reading. Percent/ETA are measured by compressed
// bytes read from the source file (the only quantity whose total is known up
// front); xz reads forward roughly proportionally to records yielded, so the
// percent is a fair approximation. Plain .xml / .gz: exact.
func snapshot(start, now time.Time, rows, readBytes, totalBytes int64) Progress {
	elapsed := now.Sub(start)
	pct := 0.0
	var etaSec int64
	if totalBytes > 0 {
		pct = float64(readBytes) / float64(totalBytes) * 100
		if rate := float64(readBytes) / elapsed.Seconds(); rate > 0 {
			etaSec = int64(float64(totalBytes-readBytes) / rate)
		}
	}
	var rateRows int64
	if elapsed.Seconds() > 0 {
		rateRows = int64(float64(rows) / elapsed.Seconds())
	}
	return Progress{
		Pct:       pct,
		Rows:      rows,
		ReadMB:    readBytes / (1 << 20),
		TotalMB:   totalBytes / (1 << 20),
		RateRowsS: rateRows,
		EtaS:      etaSec,
		ElapsedS:  int64(elapsed.Seconds()),
	}
}

// fmtDur renders seconds as m:ss (or h:mm:ss past an hour).
func fmtDur(s int64) string {
	if s < 0 {
		s = 0
	}
	h := s / 3600
	m := (s % 3600) / 60
	sec := s % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, sec)
	}
	return fmt.Sprintf("%d:%02d", m, sec)
}
