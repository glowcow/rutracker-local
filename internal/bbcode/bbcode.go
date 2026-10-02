// Package bbcode converts the rutracker BBCode body into safe HTML. Strategy:
// HTML-escape first (raw <script>/<style> in old posts becomes inert text), then
// regex passes swap BBCode for HTML. Order matters: drop [img] first, URL
// whitelist before [url] expansion, presentational tags ([size]/[color]/…) last.
package bbcode

import (
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Render returns HTML for the given BBCode input. Output is safe to drop
// into a `dangerouslySetInnerHTML` / `template.HTML` sink — no raw HTML,
// no javascript: URLs, no <img> tags.
func Render(input string) string {
	// NUL can't appear in valid XML 1.0 input, but strip it defensively:
	// the protector below delimits placeholders with NUL and must never
	// collide with attacker-controlled bytes.
	input = strings.ReplaceAll(input, "\x00", "")
	s := html.EscapeString(input)

	for _, p := range dropPatterns {
		s = p.ReplaceAllString(s, "")
	}

	// URL expansion FIRST so the href whitelist runs on the original target.
	// Generated hrefs hide behind placeholders — else later tag passes rewrite
	// bracketed tokens inside them (a broken link with a dangling tag).
	var prot protector
	s = urlWithText.ReplaceAllStringFunc(s, func(m string) string {
		return expandURLWithText(m, &prot)
	})
	s = urlInline.ReplaceAllStringFunc(s, func(m string) string {
		return expandURLInline(m, &prot)
	})

	// Block-level structures before inline tags so their innards get the
	// inline pass too.
	s = spoilerTitled.ReplaceAllString(s, `<details><summary>$1</summary>$2</details>`)
	s = spoilerPlain.ReplaceAllString(s, `<details><summary>Спойлер</summary>$1</details>`)
	s = quoteAuthored.ReplaceAllString(s, `<blockquote><cite>$1</cite>$2</blockquote>`)
	s = quotePlain.ReplaceAllString(s, `<blockquote>$1</blockquote>`)
	s = listBlock.ReplaceAllStringFunc(s, expandList)

	for _, p := range simpleTagPatterns {
		s = p.re.ReplaceAllString(s, p.repl)
	}

	for _, re := range stripTagPatterns {
		s = re.ReplaceAllString(s, "")
	}

	s = hrPattern.ReplaceAllString(s, "<hr>")
	s = brPattern.ReplaceAllString(s, "<br>")

	// Source line breaks → <br>, done last. Trade-off: we may paste <br> inside
	// <pre>/<code> rather than parse nested context — rare in practice.
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\n", "<br>")

	return prot.restore(s)
}

// protector hides already-generated HTML fragments from later regex passes
// behind NUL-delimited placeholders (the input is NUL-free — see Render).
type protector struct{ frags []string }

func (p *protector) hide(frag string) string {
	p.frags = append(p.frags, frag)
	return "\x00" + strconv.Itoa(len(p.frags)-1) + "\x00"
}

func (p *protector) restore(s string) string {
	for i, frag := range p.frags {
		s = strings.Replace(s, "\x00"+strconv.Itoa(i)+"\x00", frag, 1)
	}
	return s
}

// --- regex tables ------------------------------------------------------------

// tagPattern pairs a compiled BBCode token matcher with its HTML replacement.
type tagPattern struct {
	re   *regexp.Regexp
	repl string
}

var (
	// [img]url[/img] and [img=...]url[/img]. We drop the whole thing because
	// the design decision is "no outbound HTTP, no broken thumbnails" (most
	// rutracker dumps are 2010-era — fastpic/imageshack/radikal hosts dead).
	imgPaired = regexp.MustCompile(`(?is)\[img(?:=[^\]]*)?\].*?\[/img\]`)
	// Some posts use a malformed self-contained form, e.g. [img=https://x/y.jpg]
	imgSelfClose = regexp.MustCompile(`(?is)\[img=[^\]]*\]`)

	dropPatterns = []*regexp.Regexp{imgPaired, imgSelfClose}

	urlWithText = regexp.MustCompile(`(?is)\[url=([^\]]+)\](.*?)\[/url\]`)
	urlInline   = regexp.MustCompile(`(?is)\[url\](.*?)\[/url\]`)

	spoilerTitled = regexp.MustCompile(`(?is)\[spoiler=(?:&#34;|&quot;|")(.*?)(?:&#34;|&quot;|")\](.*?)\[/spoiler\]`)
	spoilerPlain  = regexp.MustCompile(`(?is)\[spoiler\](.*?)\[/spoiler\]`)

	quoteAuthored = regexp.MustCompile(`(?is)\[quote=(?:&#34;|&quot;|")(.*?)(?:&#34;|&quot;|")\](.*?)\[/quote\]`)
	quotePlain    = regexp.MustCompile(`(?is)\[quote\](.*?)\[/quote\]`)

	listBlock     = regexp.MustCompile(`(?is)\[list(?:=[^\]]*)?\](.*?)\[/list\]`)
	listItemSplit = regexp.MustCompile(`(?i)\[\*\]`)

	hrPattern = regexp.MustCompile(`(?i)\[hr\]`)
	brPattern = regexp.MustCompile(`(?i)\[br\]`)

	simpleTags = map[string]string{
		"b":    "strong",
		"i":    "em",
		"u":    "u",
		"s":    "s",
		"pre":  "pre",
		"code": "code",
	}

	stripTags = []string{"size", "color", "font", "align", "center"}

	// Compiled once at init — Render runs per torrent-detail request, and
	// recompiling ~22 patterns per call was its dominant fixed cost.
	simpleTagPatterns = func() []tagPattern {
		out := make([]tagPattern, 0, len(simpleTags)*2)
		for bb, ht := range simpleTags {
			out = append(out,
				tagPattern{regexp.MustCompile(`(?is)\[` + bb + `\]`), "<" + ht + ">"},
				tagPattern{regexp.MustCompile(`(?is)\[/` + bb + `\]`), "</" + ht + ">"},
			)
		}
		return out
	}()

	stripTagPatterns = func() []*regexp.Regexp {
		out := make([]*regexp.Regexp, 0, len(stripTags)*2)
		for _, t := range stripTags {
			out = append(out,
				regexp.MustCompile(`(?is)\[`+t+`(?:=[^\]]*)?\]`),
				regexp.MustCompile(`(?is)\[/`+t+`\]`),
			)
		}
		return out
	}()

	allowedSchemes = map[string]bool{
		"http":   true,
		"https":  true,
		"magnet": true,
		"ftp":    true,
	}
)

// --- helpers -----------------------------------------------------------------

func expandURLWithText(match string, prot *protector) string {
	m := urlWithText.FindStringSubmatch(match)
	if len(m) != 3 {
		return match
	}
	href := html.UnescapeString(m[1]) // m[1] came through html.EscapeString
	text := m[2]
	if !urlSchemeAllowed(href) {
		return text
	}
	// Only the opening tag is hidden — the link TEXT stays visible so
	// inline tags inside it ([url=...]see [b]this[/b][/url]) still expand.
	open := `<a href="` + html.EscapeString(href) + `" target="_blank" rel="noopener noreferrer">`
	return prot.hide(open) + text + `</a>`
}

func expandURLInline(match string, prot *protector) string {
	m := urlInline.FindStringSubmatch(match)
	if len(m) != 2 {
		return match
	}
	href := html.UnescapeString(m[1])
	if !urlSchemeAllowed(href) {
		return m[1]
	}
	// The whole anchor is hidden: the visible text IS the URL, and a
	// bracketed token in it must render verbatim, not as formatting.
	escaped := html.EscapeString(href)
	return prot.hide(`<a href="` + escaped + `" target="_blank" rel="noopener noreferrer">` + escaped + `</a>`)
}

func urlSchemeAllowed(s string) bool {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil {
		return false
	}
	return allowedSchemes[strings.ToLower(u.Scheme)]
}

func expandList(match string) string {
	m := listBlock.FindStringSubmatch(match)
	if len(m) != 2 {
		return match
	}
	// Split on [*] markers directly — newlines haven't been converted to
	// <br> yet at this point in Render, so we can't rely on them as
	// separators.
	parts := listItemSplit.Split(m[1], -1)
	var b strings.Builder
	b.WriteString("<ul>")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		b.WriteString("<li>")
		b.WriteString(p)
		b.WriteString("</li>")
	}
	b.WriteString("</ul>")
	return b.String()
}
