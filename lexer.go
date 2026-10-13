package codemask

import (
	"strings"
)

type segKind uint8

const (
	segCode segKind = iota
	segLiteral
	segComment
)

// Placeholders. Delimiters stay, so `"<str>"` still reads as a string.
const (
	phString  = "<str>"
	phComment = "<comment>"
	phNumber  = "<num>"
	phText    = "<text>"
	phRegex   = "<re>"
	phArg     = "<arg>"
	phValue   = "<val>"
	phKey     = "<key>"
	phExpr    = "<expr>"
)

// segment is a piece of a line in its raw form. Segments never leave the
// machine; they feed the Facts comparison.
type segment struct {
	kind segKind
	text string
}

type lexedLine struct {
	masked string
	segs   []segment
}

// lexer masks a file one line at a time, carrying state across lines.
type lexer interface {
	lex(line string) lexedLine
	// clean reports whether the lexer is back at top level with nothing it
	// failed to understand. Anything else means its view of the file is wrong
	// somewhere, and the file must not be sent.
	clean() bool
}

type lineBuilder struct {
	out  strings.Builder
	segs []segment
}

func (b *lineBuilder) code(s string) {
	if s == "" {
		return
	}
	b.out.WriteString(s)
	if n := len(b.segs); n > 0 && b.segs[n-1].kind == segCode {
		b.segs[n-1].text += s
		return
	}
	b.segs = append(b.segs, segment{kind: segCode, text: s})
}

// delim writes comment syntax (`//`, `#`, `/*`) as part of the comment, so
// adding an empty comment line is not a code change.
func (b *lineBuilder) delim(s string) {
	if s == "" {
		return
	}
	b.out.WriteString(s)
	b.segs = append(b.segs, segment{kind: segComment, text: s})
}

func (b *lineBuilder) mask(kind segKind, placeholder, raw string) {
	b.out.WriteString(placeholder)
	b.segs = append(b.segs, segment{kind: kind, text: raw})
}

func (b *lineBuilder) done() lexedLine {
	return lexedLine{masked: b.out.String(), segs: b.segs}
}

// pending collects the content of a masked construct on the current line, so a
// string or comment becomes one placeholder per line however it is spelled.
type pending struct {
	buf         strings.Builder
	kind        segKind
	placeholder string
}

func (p *pending) start(kind segKind, placeholder string) {
	p.kind, p.placeholder = kind, placeholder
}

func (p *pending) add(s string) { p.buf.WriteString(s) }

func (p *pending) flush(b *lineBuilder) {
	if p.buf.Len() == 0 {
		return
	}
	b.mask(p.kind, p.placeholder, p.buf.String())
	p.buf.Reset()
}

func isDigit(c byte) bool      { return c >= '0' && c <= '9' }
func isIdentStart(c byte) bool { return c == '_' || c == '$' || c|0x20 >= 'a' && c|0x20 <= 'z' }
func isIdent(c byte) bool      { return isIdentStart(c) || isDigit(c) }
func isLetter(c byte) bool     { return c|0x20 >= 'a' && c|0x20 <= 'z' }
func isSpace(c byte) bool      { return c == ' ' || c == '\t' }

func leadingSpace(s string) string {
	return s[:len(s)-len(strings.TrimLeft(s, " \t"))]
}

func trailingSpace(s string) string {
	return s[len(strings.TrimRight(s, " \t")):]
}

// longNumber is how many digits make a number worth masking. Ports, status codes
// and sizes stay; account, document and phone numbers go.
const longNumber = 6

// digitCount counts a number literal's digits, hex digits included, ignoring
// its base prefix and separators.
func digitCount(s string) int {
	if len(s) > 2 && s[0] == '0' && strings.ContainsRune("xXbBoO", rune(s[1])) {
		s = s[2:]
	}
	n := 0
	for i := 0; i < len(s); i++ {
		if c := s[i]; isDigit(c) || c|0x20 >= 'a' && c|0x20 <= 'f' {
			n++
		}
	}
	return n
}
