package main

import (
	"regexp"
	"strings"
	"unicode"
)

const (
	mdStrong = '\x01'
	mdCode   = '\x02'
)

var (
	mdFence   = regexp.MustCompile("^\\s*(```|~~~)")
	mdRule    = regexp.MustCompile(`^\s*([-*_]\s*){3,}$`)
	mdTableHR = regexp.MustCompile(`^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$`)
	mdHeading = regexp.MustCompile(`^\s*#{1,6}\s+`)
	mdBullet  = regexp.MustCompile(`^(\s*)[-*+]\s+(\[[ xX]\]\s+)?`)
	mdQuote   = regexp.MustCompile(`^\s*>\s?`)
	mdLink    = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)
)

// orcaMarkdown turns a reply into lines for the strip: block marks become plain glyphs, strong and code spans are wrapped in mdStrong / mdCode toggles, blank lines and rules drop out.
func orcaMarkdown(s string) string {
	var out []string
	fence := false
	for _, line := range strings.Split(strings.ReplaceAll(s, "\r", ""), "\n") {
		if mdFence.MatchString(line) {
			fence = !fence
			continue
		}
		if fence {
			if t := strings.TrimRight(line, " \t"); t != "" {
				out = append(out, string(mdCode)+mdClean(t)+string(mdCode))
			}
			continue
		}
		if strings.TrimSpace(line) == "" || mdRule.MatchString(line) || strings.Contains(line, "|") && mdTableHR.MatchString(line) {
			continue
		}
		switch {
		case mdHeading.MatchString(line):
			text := strings.ReplaceAll(mdInline(mdHeading.ReplaceAllString(line, "")), string(mdStrong), "")
			out = append(out, string(mdStrong)+text+string(mdStrong))
		case strings.HasPrefix(strings.TrimSpace(line), "|"):
			cells := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
			for i, c := range cells {
				cells[i] = mdInline(strings.TrimSpace(c))
			}
			out = append(out, strings.Join(cells, " · "))
		case mdBullet.MatchString(line):
			m := mdBullet.FindStringSubmatch(line)
			out = append(out, strings.Repeat(" ", len(m[1])/2)+"• "+mdInline(line[len(m[0]):]))
		case mdQuote.MatchString(line):
			out = append(out, "│ "+mdInline(mdQuote.ReplaceAllString(line, "")))
		default:
			out = append(out, mdInline(strings.TrimSpace(line)))
		}
	}
	return strings.Join(out, "\n")
}

func mdClean(s string) string {
	return strings.Map(func(r rune) rune {
		if r == mdStrong || r == mdCode {
			return -1
		}
		return r
	}, s)
}

// mdInline styles one line: `code` stays literal, **strong** / __strong__ toggle mdStrong, *em* / _em_ lose their marks, links keep their text.
func mdInline(s string) string {
	s = mdClean(s)
	var b strings.Builder
	parts := strings.Split(s, "`")
	for i, p := range parts {
		if i%2 == 1 && i < len(parts)-1 {
			b.WriteRune(mdCode)
			b.WriteString(p)
			b.WriteRune(mdCode)
			continue
		}
		if i%2 == 1 {
			b.WriteString("`")
		}
		b.WriteString(mdEmphasis(mdLink.ReplaceAllString(p, "$1")))
	}
	return b.String()
}

func mdEmphasis(s string) string {
	for _, m := range []string{"**", "__"} {
		if strings.Count(s, m) >= 2 {
			n := strings.Count(s, m) / 2 * 2
			s = strings.Replace(s, m, string(mdStrong), n)
		}
	}
	rs := []rune(s)
	var b strings.Builder
	for i, r := range rs {
		if r == '*' || r == '_' {
			prev, next := ' ', ' '
			if i > 0 {
				prev = rs[i-1]
			}
			if i < len(rs)-1 {
				next = rs[i+1]
			}
			word := func(c rune) bool { return unicode.IsLetter(c) || unicode.IsDigit(c) }
			if word(prev) && word(next) || !word(prev) && !word(next) && prev != mdStrong && next != mdStrong {
				b.WriteRune(r)
				continue
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
