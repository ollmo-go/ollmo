// Package htmltext converts a fetched HTML page to plain text for ingestion.
// It is intentionally small: strip script/style/head noise, turn block tags
// into line breaks, decode entities. Deep cleaning (boilerplate removal,
// readability heuristics) stays out of scope.
package htmltext

import (
	"html"
	"regexp"
	"strings"
)

var (
	scriptRe = regexp.MustCompile(`(?is)<(script|style|noscript|svg|head|iframe)[^>]*>.*?</(script|style|noscript|svg|head|iframe)>`)
	commentRe = regexp.MustCompile(`(?s)<!--.*?-->`)
	blockRe   = regexp.MustCompile(`(?i)</?(p|div|br|li|tr|h[1-6]|section|article|blockquote|pre|table|ul|ol|dl|dt|dd|hr|figcaption|figure)[^>]*>`)
	tagRe     = regexp.MustCompile(`(?s)<[^>]+>`)
	titleRe   = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	spaceRe   = regexp.MustCompile(`[ \t\xa0]+`)
	blankRe   = regexp.MustCompile(`\n{3,}`)
)

// Extract returns the page title and the plain text body.
func Extract(pageHTML string) (title, text string) {
	if m := titleRe.FindStringSubmatch(pageHTML); len(m) > 1 {
		title = strings.TrimSpace(html.UnescapeString(m[1]))
	}

	s := scriptRe.ReplaceAllString(pageHTML, " ")
	s = commentRe.ReplaceAllString(s, " ")
	s = blockRe.ReplaceAllString(s, "\n")
	s = tagRe.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = spaceRe.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, " \n", "\n")
	s = blankRe.ReplaceAllString(s, "\n\n")
	return title, strings.TrimSpace(s)
}
