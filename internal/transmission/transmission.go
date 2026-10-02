// Package transmission is a minimal Transmission RPC client — just enough to
// push a magnet link into the daemon's download queue (torrent-add).
package transmission

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// ErrDuplicate — the daemon already has this torrent (result arguments carry
// "torrent-duplicate" instead of "torrent-added"). Not a failure; the caller
// surfaces it as "already queued".
var ErrDuplicate = errors.New("torrent already added")

// Client talks to one Transmission RPC endpoint. The session id is cached and
// refreshed on the 409 the daemon returns when it's missing/rotated.
type Client struct {
	rpcURL string
	host   string // Host header override (matches rpc-host-whitelist)
	user   string
	pass   string
	http   *http.Client

	mu        sync.Mutex
	sessionID string
}

// New builds a client. host overrides the Host header (Transmission's
// rpc-host-whitelist checks it); user/pass are optional Basic auth.
func New(rpcURL, host, user, pass string) *Client {
	return &Client{
		rpcURL: rpcURL,
		host:   host,
		user:   user,
		pass:   pass,
		http:   &http.Client{Timeout: 8 * time.Second},
	}
}

// Add queues a magnet. Returns the torrent name on success, ErrDuplicate if it
// was already present, or a wrapped error on any RPC/transport failure.
func (c *Client) Add(ctx context.Context, magnet string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"method":    "torrent-add",
		"arguments": map[string]any{"filename": magnet},
	})

	// First call may 409 to hand back a fresh session id; retry once with it.
	for attempt := 0; attempt < 2; attempt++ {
		data, status, err := c.post(ctx, body)
		if err != nil {
			return "", err
		}
		if status == http.StatusConflict {
			continue // session id refreshed inside post(); retry
		}
		if status != http.StatusOK {
			return "", fmt.Errorf("transmission rpc: http %d", status)
		}

		var out struct {
			Result    string `json:"result"`
			Arguments struct {
				Added *struct {
					Name string `json:"name"`
				} `json:"torrent-added"`
				Duplicate *struct {
					Name string `json:"name"`
				} `json:"torrent-duplicate"`
			} `json:"arguments"`
		}
		if err := json.Unmarshal(data, &out); err != nil {
			return "", fmt.Errorf("transmission rpc: decode: %w", err)
		}
		if out.Result != "success" {
			return "", fmt.Errorf("transmission rpc: %s", out.Result)
		}
		if out.Arguments.Duplicate != nil {
			return out.Arguments.Duplicate.Name, ErrDuplicate
		}
		if out.Arguments.Added != nil {
			return out.Arguments.Added.Name, nil
		}
		return "", nil
	}
	return "", errors.New("transmission rpc: session handshake failed")
}

// Ping is a cheap reachability check: any HTTP answer (200 or the 409 session
// handshake) means the daemon is up. A transport error means it's unreachable.
func (c *Client) Ping(ctx context.Context) error {
	_, status, err := c.post(ctx, []byte(`{"method":"session-get"}`))
	if err != nil {
		return err
	}
	if status == http.StatusOK || status == http.StatusConflict {
		return nil
	}
	return fmt.Errorf("transmission rpc: http %d", status)
}

// post does one RPC round-trip. On 409 it captures the refreshed session id and
// returns the status so Add can retry; otherwise it returns the body.
func (c *Client) post(ctx context.Context, body []byte) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.rpcURL, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.host != "" {
		req.Host = c.host // Host header, not req.Header (net/http reads req.Host)
	}
	if c.user != "" {
		req.SetBasicAuth(c.user, c.pass)
	}
	c.mu.Lock()
	sid := c.sessionID
	c.mu.Unlock()
	if sid != "" {
		req.Header.Set("X-Transmission-Session-Id", sid)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		if fresh := resp.Header.Get("X-Transmission-Session-Id"); fresh != "" {
			c.mu.Lock()
			c.sessionID = fresh
			c.mu.Unlock()
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, resp.StatusCode, nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return data, resp.StatusCode, nil
}
