package peers

import (
	"errors"
	"testing"
)

func TestParsePeers(t *testing.T) {
	// Real markup captured from a logged-in topic page (cp1251 label between
	// ">" and "<b>"; here as UTF-8 which is fine — the regex ignores it).
	logged := []byte(`<div class="attach">
		<span class="seed">Сиды:&nbsp; <b>12</b></span>
		<span class="leech">Личи:&nbsp; <b>1</b></span></div>`)
	got, err := parsePeers(logged)
	if err != nil {
		t.Fatalf("logged-in: unexpected err %v", err)
	}
	if got.Seeders != 12 || got.Leechers != 1 {
		t.Errorf("logged-in: got %+v, want {12 1}", got)
	}

	// A dead-but-confirmed torrent keeps the block with zeros — must NOT be
	// mistaken for the anonymous (logged-out) page.
	dead := []byte(`<span class="seed">Сиды:&nbsp; <b>0</b></span>
		<span class="leech">Личи:&nbsp; <b>0</b></span>`)
	got, err = parsePeers(dead)
	if err != nil {
		t.Fatalf("dead: unexpected err %v", err)
	}
	if got.Seeders != 0 || got.Leechers != 0 {
		t.Errorf("dead: got %+v, want {0 0}", got)
	}

	// Anonymous view — the peer block is absent entirely.
	anon := []byte(`<div class="post_body">Гостевая страница без блока пиров</div>`)
	if _, err := parsePeers(anon); !errors.Is(err, ErrAnon) {
		t.Errorf("anon: got err %v, want ErrAnon", err)
	}
}
