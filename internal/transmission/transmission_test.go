package transmission

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// daemon fakes the RPC: the first call of a session gets the 409 handshake,
// then torrent-add answers with reply.
func daemon(t *testing.T, reply string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("X-Transmission-Session-Id") != "sid-1" {
			w.Header().Set("X-Transmission-Session-Id", "sid-1")
			w.WriteHeader(http.StatusConflict)
			return
		}
		if u, p, ok := r.BasicAuth(); !ok || u != "u" || p != "p" || r.Host != "tx.lan" {
			http.Error(w, "unauthorised", http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"filename":"magnet:?xt=1"`) && !strings.Contains(string(body), "session-get") {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestAdd(t *testing.T) {
	srv, calls := daemon(t, `{"result":"success","arguments":{"torrent-added":{"name":"Film"}}}`)
	c := New(srv.URL, "tx.lan", "u", "p")
	name, err := c.Add(t.Context(), "magnet:?xt=1")
	if err != nil || name != "Film" || calls.Load() != 2 {
		t.Fatalf("add: %q %v after %d calls", name, err, calls.Load())
	}
	// The session id is kept: the next call needs no handshake.
	if _, err := c.Add(t.Context(), "magnet:?xt=1"); err != nil || calls.Load() != 3 {
		t.Errorf("second add: %v after %d calls", err, calls.Load())
	}
}

func TestAddDuplicate(t *testing.T) {
	srv, _ := daemon(t, `{"result":"success","arguments":{"torrent-duplicate":{"name":"Film"}}}`)
	name, err := New(srv.URL, "tx.lan", "u", "p").Add(t.Context(), "magnet:?xt=1")
	if !errors.Is(err, ErrDuplicate) || name != "Film" {
		t.Errorf("got %q %v, want ErrDuplicate", name, err)
	}
}

func TestAddFailures(t *testing.T) {
	srv, _ := daemon(t, `{"result":"invalid or corrupt torrent file"}`)
	if _, err := New(srv.URL, "tx.lan", "u", "p").Add(t.Context(), "magnet:?xt=1"); err == nil || !strings.Contains(err.Error(), "corrupt") {
		t.Errorf("rpc failure: %v", err)
	}
	if _, err := New(srv.URL, "tx.lan", "u", "wrong").Add(t.Context(), "magnet:?xt=1"); err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("auth failure: %v", err)
	}
	srv.Close()
	if _, err := New(srv.URL, "", "", "").Add(t.Context(), "magnet:?xt=1"); err == nil {
		t.Error("no error from a dead daemon")
	}
}

func TestPing(t *testing.T) {
	srv, _ := daemon(t, `{"result":"success"}`)
	if err := New(srv.URL, "tx.lan", "u", "p").Ping(t.Context()); err != nil {
		t.Errorf("ping: %v", err)
	}
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer down.Close()
	if err := New(down.URL, "", "", "").Ping(t.Context()); err == nil {
		t.Error("ping passed on a 502")
	}
}
