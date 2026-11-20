// Package bbcode turns a description's BBCode into safe HTML: escape all, then
// expand tags — images out first, URLs checked before [url], styles last.
package bbcode

import (
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Render returns HTML safe to inject as is: no raw HTML, no javascript: URLs,
// no images.
func Render(input string) string {
	// NUL delimits the placeholders below, so none may come from the input.
	input = strings.ReplaceAll(input, "\x00", "")
	text := html.EscapeString(input)

	for _, p := range dropPatterns {
		text = p.ReplaceAllString(text, "")
	}

	// URLs first, so the scheme check sees the original target; the links hide
	// behind placeholders from the later passes.
	var prot protector
	text = urlWithText.ReplaceAllStringFunc(text, func(m string) string {
		return expandURLWithText(m, &prot)
	})
	text = urlInline.ReplaceAllStringFunc(text, func(m string) string {
		return expandURLInline(m, &prot)
	})

	// Block-level structures before inline tags so their innards get the
	// inline pass too.
	text = spoilerTitled.ReplaceAllString(text, `<details><summary>$1</summary>$2</details>`)
	text = spoilerPlain.ReplaceAllString(text, `<details><summary>Спойлер</summary>$1</details>`)
	text = quoteAuthored.ReplaceAllString(text, `<blockquote><cite>$1</cite>$2</blockquote>`)
	text = quotePlain.ReplaceAllString(text, `<blockquote>$1</blockquote>`)
	text = listBlock.ReplaceAllStringFunc(text, expandList)

	for _, p := range simpleTagPatterns {
		text = p.re.ReplaceAllString(text, p.repl)
	}

	for _, re := range stripTagPatterns {
		text = re.ReplaceAllString(text, "")
	}

	text = hrPattern.ReplaceAllString(text, "<hr>")
	text = brPattern.ReplaceAllString(text, "<br>")

	// Source line breaks → <br>, done last. Trade-off: we may paste <br> inside
	// <pre>/<code> rather than parse nested context — rare in practice.
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\n", "<br>")

	return prot.restore(text)
}

// protector hides already-generated HTML fragments from later regex passes
// behind NUL-delimited placeholders (the input is NUL-free — see Render).
type protector struct{ frags []string }

func (p *protector) hide(frag string) string {
	p.frags = append(p.frags, frag)
	return "\x00" + strconv.Itoa(len(p.frags)-1) + "\x00"
}

func (p *protector) restore(text string) string {
	for i, frag := range p.frags {
		text = strings.Replace(text, "\x00"+strconv.Itoa(i)+"\x00", frag, 1)
	}
	return text
}

// tagPattern pairs a compiled BBCode token matcher with its HTML replacement.
type tagPattern struct {
	re   *regexp.Regexp
	repl string
}

var (
	// Images go: no outbound requests, and the 2010-era image hosts are dead.
	imgPaired = regexp.MustCompile(`(?is)\[img(?:=[^\]]*)?\].*?\[/img\]`)
	// A malformed self-closed form: [img=https://x/y.jpg].
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
		for _, tag := range stripTags {
			out = append(out,
				regexp.MustCompile(`(?is)\[`+tag+`(?:=[^\]]*)?\]`),
				regexp.MustCompile(`(?is)\[/`+tag+`\]`),
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

func expandURLWithText(match string, prot *protector) string {
	groups := urlWithText.FindStringSubmatch(match)
	if len(groups) != 3 {
		return match
	}
	href := html.UnescapeString(groups[1]) // escaped by Render
	text := groups[2]
	if !urlSchemeAllowed(href) {
		return text
	}
	// Only the opening tag is hidden — the link TEXT stays visible so
	// inline tags inside it ([url=...]see [b]this[/b][/url]) still expand.
	open := `<a href="` + html.EscapeString(href) + `" target="_blank" rel="noopener noreferrer">`
	return prot.hide(open) + text + `</a>`
}

func expandURLInline(match string, prot *protector) string {
	groups := urlInline.FindStringSubmatch(match)
	if len(groups) != 2 {
		return match
	}
	href := html.UnescapeString(groups[1])
	if !urlSchemeAllowed(href) {
		return groups[1]
	}
	// The whole anchor is hidden: the visible text IS the URL, and a
	// bracketed token in it must render verbatim, not as formatting.
	escaped := html.EscapeString(href)
	return prot.hide(`<a href="` + escaped + `" target="_blank" rel="noopener noreferrer">` + escaped + `</a>`)
}

func urlSchemeAllowed(s string) bool {
	parsed, err := url.Parse(strings.TrimSpace(s))
	if err != nil {
		return false
	}
	return allowedSchemes[strings.ToLower(parsed.Scheme)]
}

func expandList(match string) string {
	groups := listBlock.FindStringSubmatch(match)
	if len(groups) != 2 {
		return match
	}
	// Split on [*]: newlines are not <br> yet at this point.
	parts := listItemSplit.Split(groups[1], -1)
	var out strings.Builder
	out.WriteString("<ul>")
	for _, item := range parts {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out.WriteString("<li>")
		out.WriteString(item)
		out.WriteString("</li>")
	}
	out.WriteString("</ul>")
	return out.String()
}
