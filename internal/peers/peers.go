// Package peers scrapes seeders/leechers for one rutracker topic. Counts are
// visible only to a logged-in session (guests get no peer block; /scrape is 403,
// no public API). Flow: hold a bb_session cookie, GET the topic, parse the block;
// if the block is gone the cookie is dead → re-login (single-flight) and retry.
// Every request is rate-limited to stay gentle on the account.
package peers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// A recent desktop UA — the guest/login probes showed Cloudflare serves 200
// (no JS challenge) to a normal browser UA, unlike a bare curl default.
const userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) Chrome/120.0 Safari/537.36"

// Sentinel errors the endpoint maps to a UI state.
var (
	// ErrAnon — the page loaded but had no peer block: the cookie is invalid
	// (we're seeing the anonymous view). Distinct from a real 0/0, which keeps
	// the block. Every id we scrape is a torrent, so a missing block always
	// means "not authorised", never "not a torrent".
	ErrAnon = errors.New("peers: anonymous page (session invalid)")
	// ErrAuth — login itself failed (bad credentials or a captcha wall).
	ErrAuth = errors.New("peers: login failed")
	// ErrUnavailable — rutracker unreachable (network / Cloudflare / non-200).
	ErrUnavailable = errors.New("peers: rutracker unavailable")
)

// Stat is a single topic's peer counts.
type Stat struct {
	Seeders  int
	Leechers int
}

// PersistFunc stores a freshly-minted cookie (e.g. into Postgres) so it
// survives restarts and isn't re-minted needlessly.
type PersistFunc func(ctx context.Context, cookie string) error

// Client scrapes peers for one account. Safe for concurrent use.
type Client struct {
	base    string
	user    string
	pass    string
	http    *http.Client
	lim     *limiter
	persist PersistFunc

	mu     sync.RWMutex
	cookie string

	// login collapses concurrent re-logins (a burst of ErrAnon fetches) into
	// one request keyed by a constant.
	login singleflight.Group
}

// New builds a client. cookie is the last-known bb_session (may be empty);
// gap is the minimum spacing between outbound requests. persist may be nil.
func New(base, user, pass, cookie string, gap time.Duration, persist PersistFunc) *Client {
	return &Client{
		base:    strings.TrimRight(base, "/"),
		user:    user,
		pass:    pass,
		cookie:  cookie,
		lim:     &limiter{gap: gap},
		persist: persist,
		http: &http.Client{
			// Don't follow redirects: login.php answers 302 + Set-Cookie on
			// success and we must read that cookie off the redirect itself.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Timeout: 20 * time.Second,
		},
	}
}

// Fetch returns the peer counts for topicID, re-logging in once if the cached
// session has expired.
func (c *Client) Fetch(ctx context.Context, topicID int64) (Stat, error) {
	if err := c.lim.wait(ctx); err != nil {
		return Stat{}, err
	}
	stat, err := c.fetchOnce(ctx, topicID)
	if !errors.Is(err, ErrAnon) {
		return stat, err
	}
	// Cookie dead → re-login (single-flight) and retry exactly once.
	if _, lerr := c.ensureLogin(ctx); lerr != nil {
		return Stat{}, lerr
	}
	if err := c.lim.wait(ctx); err != nil {
		return Stat{}, err
	}
	return c.fetchOnce(ctx, topicID)
}

func (c *Client) fetchOnce(ctx context.Context, topicID int64) (Stat, error) {
	c.mu.RLock()
	cookie := c.cookie
	c.mu.RUnlock()

	u := fmt.Sprintf("%s/forum/viewtopic.php?t=%d", c.base, topicID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Stat{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	req.Header.Set("User-Agent", userAgent)
	if cookie != "" {
		req.Header.Set("Cookie", "bb_session="+cookie)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return Stat{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Stat{}, fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	}
	// Cap the read: topic pages are tens of KB; this bounds a hostile/broken
	// response. Markers are ASCII so we parse the raw cp1251 bytes directly.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return Stat{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return parsePeers(body)
}

// Observed markup: <span class="seed">Сиды:&nbsp; <b>12</b></span> (same for
// leech). The cyrillic label between ">" and "<b>" contains no "<", so a
// lazy match up to the first <b> is safe. (?s) lets the block span newlines.
var (
	seedRe  = regexp.MustCompile(`(?s)<span class="seed">.*?<b>(\d+)</b>`)
	leechRe = regexp.MustCompile(`(?s)<span class="leech">.*?<b>(\d+)</b>`)
)

func parsePeers(body []byte) (Stat, error) {
	sm := seedRe.FindSubmatch(body)
	if sm == nil {
		// No seed block at all → anonymous view → session invalid.
		return Stat{}, ErrAnon
	}
	s, _ := strconv.Atoi(string(sm[1]))
	l := 0
	if lm := leechRe.FindSubmatch(body); lm != nil {
		l, _ = strconv.Atoi(string(lm[1]))
	}
	return Stat{Seeders: s, Leechers: l}, nil
}

func (c *Client) ensureLogin(ctx context.Context) (string, error) {
	v, err, _ := c.login.Do("login", func() (any, error) {
		return c.doLogin(ctx)
	})
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

func (c *Client) doLogin(ctx context.Context) (string, error) {
	form := url.Values{}
	form.Set("login_username", c.user)
	form.Set("login_password", c.pass)
	// The submit button field must be present for login.php to take the login
	// branch; its value is "вход" in cp1251 (url.Values percent-encodes the
	// raw bytes, which the server decodes back to cp1251).
	form.Set("login", "\xe2\xf5\xee\xe4")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.base+"/forum/login.php", strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	// Drain so the connection can be reused.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	cookie := bbSession(resp.Cookies())
	if cookie == "" {
		// No session cookie set → wrong creds or a captcha wall. Either way a
		// human must intervene; surface as auth failure (don't hammer).
		return "", ErrAuth
	}

	c.mu.Lock()
	c.cookie = cookie
	c.mu.Unlock()
	if c.persist != nil {
		if err := c.persist(ctx, cookie); err != nil {
			slog.Warn("persist rutracker session", "err", err)
		}
	}
	return cookie, nil
}

func bbSession(cookies []*http.Cookie) string {
	for _, ck := range cookies {
		if ck.Name == "bb_session" && ck.Value != "" {
			return ck.Value
		}
	}
	return ""
}

// limiter spaces calls by at least `gap`, honouring context cancellation.
type limiter struct {
	mu   sync.Mutex
	gap  time.Duration
	next time.Time
}

func (l *limiter) wait(ctx context.Context) error {
	l.mu.Lock()
	now := time.Now()
	var d time.Duration
	if now.Before(l.next) {
		d = l.next.Sub(now)
		l.next = l.next.Add(l.gap)
	} else {
		l.next = now.Add(l.gap)
	}
	l.mu.Unlock()

	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
