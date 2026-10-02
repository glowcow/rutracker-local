package server

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/glowcow/rutracker-local/internal/config"
	"github.com/glowcow/rutracker-local/internal/ingest"
)

// parseHub is an in-memory pub/sub for the current parse run's events. It keeps
// the full event log of the run so a late subscriber (page reload, another
// device) replays everything so far, then streams live. Single-flight means at
// most one run at a time, so a fresh run resets the buffer.
type parseHub struct {
	mu     sync.Mutex
	buf    []ingest.Event
	subs   map[int]chan ingest.Event
	nextID int
}

func newParseHub() *parseHub {
	return &parseHub{subs: make(map[int]chan ingest.Event)}
}

// reset clears the buffered log — called at the start of a new run.
func (h *parseHub) reset() {
	h.mu.Lock()
	h.buf = nil
	h.mu.Unlock()
}

// publish appends an event to the run log and fans it out to live subscribers.
// Matches the ingest.Emitter signature. Sends are non-blocking: a subscriber
// whose buffer is full (a stalled SSE client) drops the event rather than
// stalling the parse.
func (h *parseHub) publish(ev ingest.Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.buf = append(h.buf, ev)
	for _, ch := range h.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

// subscribe registers a live listener. It returns a snapshot of everything
// buffered so far (replay it first), the live channel, and an unsubscribe func.
// Snapshot + registration happen under one lock so no event can slip between
// them.
func (h *parseHub) subscribe() (replay []ingest.Event, ch chan ingest.Event, cancel func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	replay = make([]ingest.Event, len(h.buf))
	copy(replay, h.buf)
	id := h.nextID
	h.nextID++
	ch = make(chan ingest.Event, 512)
	h.subs[id] = ch
	var once sync.Once
	cancel = func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subs, id)
			close(ch)
			h.mu.Unlock()
		})
	}
	return replay, ch, cancel
}

// parseController owns the single-flight guard and drives ingest.Run for the
// in-app admin endpoint. The run executes on a detached context derived from
// the server lifetime, so it survives the triggering HTTP request returning but
// is still cancelled on server shutdown (→ ingest marks the run failed).
type parseController struct {
	baseCtx context.Context
	cfg     config.Config
	hub     *parseHub
	running atomic.Bool
}

func newParseController(baseCtx context.Context, cfg config.Config) *parseController {
	return &parseController{baseCtx: baseCtx, cfg: cfg, hub: newParseHub()}
}

// start launches a parse if none is in flight. Returns false when one is
// already running (single-flight → the handler answers 409).
func (c *parseController) start(opts ingest.Options) bool {
	if !c.running.CompareAndSwap(false, true) {
		return false
	}
	c.hub.reset()
	go func() {
		defer c.running.Store(false)
		if _, err := ingest.Run(c.baseCtx, c.cfg.DatabaseURL, opts, c.hub.publish); err != nil {
			slog.Error("parse run failed", "err", err)
		}
	}()
	return true
}

func (c *parseController) isRunning() bool { return c.running.Load() }
