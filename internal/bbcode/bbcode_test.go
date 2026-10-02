package bbcode

import (
	"strings"
	"testing"
)

func TestRender_SimpleTags(t *testing.T) {
	cases := map[string]string{
		"[b]bold[/b]":         "<strong>bold</strong>",
		"[i]italic[/i]":       "<em>italic</em>",
		"[u]under[/u]":        "<u>under</u>",
		"[s]strike[/s]":       "<s>strike</s>",
		"[pre]code[/pre]":     "<pre>code</pre>",
		"[code]inline[/code]": "<code>inline</code>",
	}
	for in, want := range cases {
		got := Render(in)
		if got != want {
			t.Errorf("Render(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRender_DropsImg(t *testing.T) {
	cases := []string{
		`[img]https://dead.example/cover.jpg[/img]`,
		`[img=right]https://dead.example/cover.jpg[/img]`,
		`Before [img]https://x/y.png[/img] after`,
		`[img=https://x/y.jpg]`, // self-closing variant
	}
	for _, in := range cases {
		got := Render(in)
		if strings.Contains(got, "img") {
			t.Errorf("Render(%q) still has img: %q", in, got)
		}
		if strings.Contains(got, "https://") && !strings.Contains(in, "after") {
			t.Errorf("Render(%q) leaked URL: %q", in, got)
		}
	}
	got := Render(`Before [img]https://x/y.png[/img] after`)
	if got != "Before  after" {
		t.Errorf("Img drop changed surrounding text: got %q", got)
	}
}

func TestRender_URLs_WhitelistAndEscape(t *testing.T) {
	got := Render(`[url=https://rutracker.org/forum/viewtopic.php?t=1]Click[/url]`)
	want := `<a href="https://rutracker.org/forum/viewtopic.php?t=1" target="_blank" rel="noopener noreferrer">Click</a>`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// javascript: URL → schemes not whitelisted → keep text, drop link
	got = Render(`[url=javascript:alert(1)]click me[/url]`)
	if strings.Contains(got, "javascript:") || strings.Contains(got, "<a") {
		t.Errorf("javascript URL got through: %q", got)
	}
	if !strings.Contains(got, "click me") {
		t.Errorf("text content dropped: %q", got)
	}

	// Inline form [url]https://...[/url]
	got = Render(`[url]https://example.com[/url]`)
	if !strings.Contains(got, `<a href="https://example.com"`) {
		t.Errorf("inline URL not expanded: %q", got)
	}
}

func TestRender_EscapesRawHTML(t *testing.T) {
	got := Render(`<script>alert(1)</script>`)
	if strings.Contains(got, "<script>") {
		t.Errorf("raw <script> survived: %q", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Errorf("expected escaped script tag, got %q", got)
	}
}

func TestRender_Spoiler(t *testing.T) {
	got := Render(`[spoiler="Лог EAC"]bla[/spoiler]`)
	if !strings.Contains(got, `<details><summary>Лог EAC</summary>bla</details>`) {
		t.Errorf("titled spoiler malformed: %q", got)
	}
	got = Render(`[spoiler]hidden[/spoiler]`)
	if !strings.Contains(got, `<details><summary>Спойлер</summary>hidden</details>`) {
		t.Errorf("plain spoiler malformed: %q", got)
	}
}

func TestRender_QuoteAndList(t *testing.T) {
	got := Render(`[quote="Author"]said this[/quote]`)
	if !strings.Contains(got, `<blockquote><cite>Author</cite>said this</blockquote>`) {
		t.Errorf("authored quote: %q", got)
	}

	got = Render("[list][*]one\n[*]two\n[*]three[/list]")
	if !strings.Contains(got, "<ul>") || !strings.Contains(got, "<li>one</li>") || !strings.Contains(got, "<li>three</li>") {
		t.Errorf("list malformed: %q", got)
	}
}

func TestRender_StripsPresentational(t *testing.T) {
	cases := map[string]string{
		"[size=24]big[/size]":       "big",
		"[color=red]red[/color]":    "red",
		"[font=Arial]styled[/font]": "styled",
		"[align=center]mid[/align]": "mid",
		"[center]mid[/center]":      "mid",
	}
	for in, want := range cases {
		got := Render(in)
		if got != want {
			t.Errorf("Render(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRender_Newlines(t *testing.T) {
	got := Render("line1\nline2\nline3")
	if got != "line1<br>line2<br>line3" {
		t.Errorf("newlines: %q", got)
	}
	got = Render("crlf\r\nhere")
	if got != "crlf<br>here" {
		t.Errorf("crlf: %q", got)
	}
}

func TestRender_NestedBoldInsideUrl(t *testing.T) {
	got := Render(`[url=https://example.com][b]Bold link[/b][/url]`)
	if !strings.Contains(got, `<a href="https://example.com"`) {
		t.Errorf("URL not rendered: %q", got)
	}
	if !strings.Contains(got, "<strong>Bold link</strong>") {
		t.Errorf("nested [b] not rendered: %q", got)
	}
}

// Bracketed tokens inside a URL must survive verbatim — the simple/strip
// passes used to rewrite them inside the already-generated href
// ([url]https://x.com/[b][/url] → href="https://x.com/<strong>").
func TestRender_BracketTokensInsideHrefSurvive(t *testing.T) {
	got := Render(`[url]https://example.com/[b]path?x=[size=12][/url]`)
	if !strings.Contains(got, `href="https://example.com/[b]path?x=[size=12]"`) {
		t.Errorf("href was rewritten by tag passes: %q", got)
	}
	if strings.Contains(got, "<strong>") {
		t.Errorf("[b] inside inline URL expanded to formatting: %q", got)
	}

	// Named form (href can't contain "]" — the capture stops there, a
	// pre-existing regex limit): href protected, link TEXT still gets
	// inline tags.
	got = Render(`[url=https://example.com/dir][b]жирная ссылка[/b][/url]`)
	if !strings.Contains(got, `href="https://example.com/dir"`) {
		t.Errorf("named-form href was rewritten: %q", got)
	}
	if !strings.Contains(got, "<strong>жирная ссылка</strong>") {
		t.Errorf("link text lost inline formatting: %q", got)
	}
}

// NUL bytes are stripped before the placeholder pass — crafted input must
// not be able to collide with protector tokens and swap fragments around.
func TestRender_NulBytesCannotForgeplaceholders(t *testing.T) {
	got := Render("\x000\x00 [url]https://example.com[/url]")
	if strings.Contains(got, "\x00") {
		t.Errorf("NUL leaked into output: %q", got)
	}
	if !strings.Contains(got, `href="https://example.com"`) {
		t.Errorf("URL not rendered: %q", got)
	}
	// The literal "0" from the input must remain plain text, not become an
	// anchor via placeholder collision.
	if strings.Count(got, "<a ") != 1 {
		t.Errorf("expected exactly one anchor, got: %q", got)
	}
}

func TestRender_RealWorldFragment(t *testing.T) {
	// Compressed slice of the actual rutracker BBCode we saw in the dump.
	in := `[size=24]Amnistia / Egotrap[/size]

[img=right]http://dead.example/cover.jpg[/img]

[b]Жанр[/b]: EBM
[b]Год издания[/b]: 2011

[spoiler="Лог EAC"][pre]Exact Audio Copy V0.99[/pre][/spoiler]`

	got := Render(in)

	for _, must := range []string{
		"Amnistia / Egotrap",                  // [size] stripped, text kept
		"<strong>Жанр</strong>: EBM",          // [b] → <strong>
		"<details><summary>Лог EAC</summary>", // spoiler title
		"<pre>",                               // [pre] tag
	} {
		if !strings.Contains(got, must) {
			t.Errorf("output missing %q\nfull:\n%s", must, got)
		}
	}
	for _, forbidden := range []string{
		"img",          // image dropped entirely
		"dead.example", // url leaked through
		"[size=",       // raw bbcode left in output
		"[/spoiler]",
	} {
		if strings.Contains(got, forbidden) {
			t.Errorf("output still contains %q\nfull:\n%s", forbidden, got)
		}
	}
}
