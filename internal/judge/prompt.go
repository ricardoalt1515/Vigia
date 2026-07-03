package judge

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed prompt/system.v1.md
var systemPromptTemplate string

// BuildTranscriptBlock renders the transcript as a single, clearly
// delimited data block: <transcript>...</transcript>, with one
// <utterance speaker="..."> element per Utterance. Speaker and text are
// XML-escaped so a malicious utterance cannot forge a closing tag and
// escape the data boundary into the surrounding instructions (ADR-11).
func BuildTranscriptBlock(utterances []Utterance) string {
	var b strings.Builder
	b.WriteString("<transcript>\n")
	for _, u := range utterances {
		fmt.Fprintf(&b, "<utterance speaker=\"%s\">%s</utterance>\n", xmlEscape(u.Speaker), xmlEscape(u.Text))
	}
	b.WriteString("</transcript>")
	return b.String()
}

// xmlEscape escapes the five XML special characters. strings.NewReplacer
// processes matches left-to-right without re-scanning replaced output, so
// "&" is escaped first and never double-escapes the "&" introduced by
// escaping "<", ">", "\"", or "'".
func xmlEscape(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(s)
}
