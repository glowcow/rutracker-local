package server

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/glowcow/rutracker-local/internal/config"
	"github.com/glowcow/rutracker-local/internal/ingest"
)

// parseHub fans out the run's events and keeps them all, so a late subscriber
// replays the run so far; a new run resets it.
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

// publish keeps ev and sends it on; a stalled subscriber loses events rather
// than stalling the parse.
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

// subscribe returns the events so far and a live channel, under one lock so
// none slips between them.
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

// parseController runs one parse at a time on the server's context: it
// outlives the request and stops on shutdown, failing the run.
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
