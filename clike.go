package codemask

import (
	"regexp"
	"strings"
)

// strDelim describes one kind of string literal.
type strDelim struct {
	open, close string
	escape      bool     // backslash escapes the next character
	doubled     bool     // a doubled close is an escaped close, as in SQL's 'it''s'
	multiline   bool     // may run past the end of a line
	interp      []string // openers of embedded code, as in `${x}`
}

// langConfig describes a language in the C family: anything whose literals and
// comments are delimited by fixed tokens.
type langConfig struct {
	lineComments  []string
	blockComments [][2]string
	strings       []strDelim // longest opener first
	jsRegex       bool       // `/re/` literals, which may contain quotes
	jsx           bool       // JSX elements, whose text children are content
	heredoc       bool       // HCL's <<EOF … EOF
	dollarQuote   bool       // Postgres' $$ … $$ and $tag$ … $tag$
	blockLabels   bool       // HCL: resource "type" "name" labels are identifiers
}

type frameKind uint8

const (
	fCode frameKind = iota
	fString
	fBlockComment
	fJSXTag
	fJSXText
)

type frame struct {
	kind    frameKind
	braces  int       // fCode: { opened in this frame
	interp  bool      // fCode: an unmatched } ends this frame
	delim   *strDelim // fString
	close   string    // fBlockComment
	closing bool      // fJSXTag: a </closing> tag
}

// clike lexes a C-family language with a stack of frames, so that code inside
// a template literal, and JSX inside that code, each end where they should.
type clike struct {
	cfg           *langConfig
	stack         []frame
	prev          string // the previous significant token, for regex and JSX decisions
	dirty         bool
	cont          bool // this line ended with a backslash inside a string
	heredocMarker string
	heredocNext   string
	labels        bool // this line opens an HCL block and its labels have not ended
	b             lineBuilder
	p             pending
}

func newClike(cfg *langConfig) *clike {
	return &clike{cfg: cfg, stack: []frame{{kind: fCode}}}
}

func (l *clike) top() *frame { return &l.stack[len(l.stack)-1] }

func (l *clike) push(f frame) { l.stack = append(l.stack, f) }

func (l *clike) pop() {
	if len(l.stack) == 1 {
		l.dirty = true
		return
	}
	l.stack = l.stack[:len(l.stack)-1]
}

// suppressed reports whether output belongs to an enclosing string, as code in
// a template literal's ${…} does. The whole literal is one placeholder.
func (l *clike) suppressed() bool {
	for _, f := range l.stack {
		if f.kind == fString {
			return true
		}
	}
	return false
}

func (l *clike) emit(s string) {
	if l.suppressed() {
		l.p.add(s)
		return
	}
	l.b.code(s)
}

func (l *clike) maskTok(kind segKind, placeholder, raw string) {
	if l.suppressed() {
		l.p.add(raw)
		return
	}
	l.b.mask(kind, placeholder, raw)
}

func (l *clike) clean() bool {
	return !l.dirty && len(l.stack) == 1 && l.heredocMarker == "" && l.heredocNext == ""
}

func (l *clike) lex(line string) lexedLine {
	l.b = lineBuilder{}
	l.cont = false
	l.labels = l.cfg.blockLabels && len(l.stack) == 1 && hclBlockLine.MatchString(line)
	if l.heredocMarker != "" {
		switch {
		case strings.TrimSpace(line) == l.heredocMarker:
			l.heredocMarker = ""
			l.b.code(line)
		case line != "":
			l.b.mask(segLiteral, phString, line)
		}
		return l.b.done()
	}
	for i := 0; i < len(line); {
		switch l.top().kind {
		case fString:
			i = l.str(line, i)
		case fBlockComment:
			i = l.block(line, i)
		case fJSXTag:
			i = l.jsxTag(line, i)
		case fJSXText:
			i = l.jsxText(line, i)
		case fCode:
			i = l.code(line, i)
		default:
			l.dirty = true
			i = len(line)
		}
	}
	if t := l.top(); t.kind == fString && !t.delim.multiline && !l.cont {
		// An unterminated single-line string: something upstream was misread.
		l.dirty = true
		l.pop()
	}
	l.p.flush(&l.b)
	if l.heredocNext != "" {
		l.heredocMarker, l.heredocNext = l.heredocNext, ""
	}
	return l.b.done()
}

func (l *clike) code(line string, i int) int {
	c := line[i]
	if isSpace(c) {
		j := i
		for j < len(line) && isSpace(line[j]) {
			j++
		}
		l.emit(line[i:j])
		return j
	}
	if j, ok := l.comment(line, i); ok {
		return j
	}
	for k := range l.cfg.strings {
		if d := &l.cfg.strings[k]; strings.HasPrefix(line[i:], d.open) {
			l.openString(d)
			return i + len(d.open)
		}
	}
	if j, ok := l.special(line, i); ok {
		return j
	}
	switch {
	case isDigit(c) || c == '.' && i+1 < len(line) && isDigit(line[i+1]):
		j := i
		for j < len(line) && (isIdent(line[j]) || line[j] == '.') {
			j++
		}
		if tok := line[i:j]; digitCount(tok) >= longNumber {
			l.maskTok(segLiteral, phNumber, tok)
		} else {
			l.emit(tok)
		}
		l.prev = "0"
		return j
	case isIdentStart(c):
		j := i
		for j < len(line) && isIdent(line[j]) {
			j++
		}
		l.emit(line[i:j])
		l.prev = line[i:j]
		return j
	case c == '{':
		l.top().braces++
		l.labels = false
	case c == '}':
		if t := l.top(); t.braces == 0 && t.interp {
			l.pop()
			l.emit("}")
			l.prev = "}"
			return i + 1
		}
		if l.top().braces > 0 {
			l.top().braces--
		}
	}
	l.emit(line[i : i+1])
	l.prev = line[i : i+1]
	return i + 1
}

func (l *clike) comment(line string, i int) (int, bool) {
	rest := line[i:]
	for _, m := range l.cfg.lineComments {
		if !strings.HasPrefix(rest, m) {
			continue
		}
		if l.suppressed() {
			l.p.add(rest)
			return len(line), true
		}
		body := rest[len(m):]
		ws := leadingSpace(body)
		l.b.delim(m + ws)
		if content := strings.TrimRight(body[len(ws):], " \t"); content != "" {
			l.b.mask(segComment, phComment, content)
		}
		return len(line), true
	}
	for _, bc := range l.cfg.blockComments {
		if !strings.HasPrefix(rest, bc[0]) {
			continue
		}
		if l.suppressed() {
			l.p.add(bc[0])
		} else {
			l.b.delim(bc[0])
			l.p.start(segComment, phComment)
		}
		l.push(frame{kind: fBlockComment, close: bc[1]})
		return i + len(bc[0]), true
	}
	return i, false
}

var (
	hclBlockLine = regexp.MustCompile(`^\s*(resource|data|module|variable|output|provider)\s+"`)
	hclLabel     = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)
	dollarTag    = regexp.MustCompile(`^\$([A-Za-z_][A-Za-z0-9_]*)?\$`)
	heredocStart = regexp.MustCompile(`^<<-?([A-Za-z_][A-Za-z0-9_]*)`)
)

// special handles the language-specific constructs: dollar quotes, heredocs,
// regex literals and JSX.
func (l *clike) special(line string, i int) (int, bool) {
	c, rest := line[i], line[i:]
	switch {
	case l.cfg.dollarQuote && c == '$':
		if m := dollarTag.FindString(rest); m != "" {
			l.openString(&strDelim{open: m, close: m, multiline: true})
			return i + len(m), true
		}
	case l.cfg.heredoc && strings.HasPrefix(rest, "<<"):
		if m := heredocStart.FindStringSubmatch(rest); m != nil {
			l.emit(m[0])
			l.heredocNext = m[1]
			return i + len(m[0]), true
		}
	case l.cfg.jsRegex && c == '/' && regexAllowed(l.prev):
		if bodyEnd, end, ok := scanRegex(line, i); ok {
			if l.suppressed() {
				l.p.add(line[i:end])
			} else {
				l.b.code("/")
				l.b.mask(segLiteral, phRegex, line[i+1:bodyEnd])
				l.b.code(line[bodyEnd:end])
			}
			l.prev = `"`
			return end, true
		}
	case l.cfg.jsx && c == '<' && jsxAllowed(l.prev) && i+1 < len(line) && (isLetter(line[i+1]) || line[i+1] == '>'):
		if line[i+1] == '>' {
			l.emit("<>")
			l.push(frame{kind: fJSXText})
			return i + 2, true
		}
		l.emit("<")
		l.push(frame{kind: fJSXTag})
		return i + 1, true
	}
	return i, false
}

func (l *clike) openString(d *strDelim) {
	if l.suppressed() {
		l.p.add(d.open)
	} else {
		l.b.code(d.open)
		l.p.start(segLiteral, phString)
	}
	l.push(frame{kind: fString, delim: d})
}

func (l *clike) closeString() {
	d := l.top().delim
	l.pop()
	switch {
	case l.suppressed():
		l.p.add(d.close)
		l.prev = `"`
		return
	case l.labels && hclLabel.MatchString(l.p.buf.String()):
		l.b.code(l.p.buf.String())
		l.p.buf.Reset()
	default:
		l.p.flush(&l.b)
	}
	l.b.code(d.close)
	l.prev = `"`
}

func (l *clike) str(line string, i int) int {
	d := l.top().delim
	j := i
	for j < len(line) {
		if d.escape && line[j] == '\\' {
			if j+1 == len(line) {
				l.cont = true
				j++
				break
			}
			j += 2
			continue
		}
		if op := matchAny(line[j:], d.interp); op != "" {
			l.p.add(line[i : j+len(op)])
			l.push(frame{kind: fCode, interp: true})
			return j + len(op)
		}
		if strings.HasPrefix(line[j:], d.close) {
			if d.doubled && strings.HasPrefix(line[j+len(d.close):], d.close) {
				j += 2 * len(d.close)
				continue
			}
			l.p.add(line[i:j])
			l.closeString()
			return j + len(d.close)
		}
		j++
	}
	l.p.add(line[i:j])
	return j
}

func (l *clike) block(line string, i int) int {
	cl := l.top().close
	k := strings.Index(line[i:], cl)
	if k < 0 {
		l.p.add(line[i:])
		return len(line)
	}
	l.p.add(line[i : i+k])
	l.pop()
	if l.suppressed() {
		l.p.add(cl)
	} else {
		l.p.flush(&l.b)
		l.b.delim(cl)
	}
	return i + k + len(cl)
}

var jsxAttrDelims = map[byte]*strDelim{
	'"':  {open: `"`, close: `"`, multiline: true},
	'\'': {open: `'`, close: `'`, multiline: true},
}

// jsxTag lexes inside <Tag …> or </Tag>. Names and attribute names are code;
// attribute strings are literals; {…} is code.
func (l *clike) jsxTag(line string, i int) int {
	c := line[i]
	switch {
	case c == '"' || c == '\'':
		l.openString(jsxAttrDelims[c])
		return i + 1
	case c == '{':
		l.emit("{")
		l.push(frame{kind: fCode, interp: true})
		return i + 1
	case strings.HasPrefix(line[i:], "/>"):
		l.emit("/>")
		l.pop()
		l.endElement()
		return i + 2
	case c == '>':
		l.emit(">")
		closing := l.top().closing
		l.pop()
		if !closing {
			l.push(frame{kind: fJSXText})
			return i + 1
		}
		if l.top().kind != fJSXText {
			l.dirty = true
			return i + 1
		}
		l.pop()
		l.endElement()
		return i + 1
	case isIdent(c) || isSpace(c):
		j := i
		for j < len(line) && (isIdent(line[j]) || isSpace(line[j])) {
			j++
		}
		l.emit(line[i:j])
		return j
	default:
		l.emit(line[i : i+1])
		return i + 1
	}
}

// jsxText lexes an element's children. Text is content; tags and {…} are not.
func (l *clike) jsxText(line string, i int) int {
	switch {
	case strings.HasPrefix(line[i:], "</"):
		l.emit("</")
		l.push(frame{kind: fJSXTag, closing: true})
		return i + 2
	case strings.HasPrefix(line[i:], "<>"):
		l.emit("<>")
		l.push(frame{kind: fJSXText})
		return i + 2
	case line[i] == '<':
		l.emit("<")
		l.push(frame{kind: fJSXTag})
		return i + 1
	case line[i] == '{':
		l.emit("{")
		l.push(frame{kind: fCode, interp: true})
		return i + 1
	}
	j := i
	for j < len(line) && line[j] != '<' && line[j] != '{' {
		j++
	}
	run := line[i:j]
	core := strings.TrimSpace(run)
	if core == "" {
		l.emit(run)
		return j
	}
	l.emit(leadingSpace(run))
	l.maskTok(segLiteral, phText, core)
	l.emit(trailingSpace(run))
	return j
}

// endElement runs when a JSX element closes. Back in code, the element was an
// expression, so a following / is division and a following < is a comparison.
func (l *clike) endElement() {
	if l.top().kind == fCode {
		l.prev = ")"
	}
}

func matchAny(s string, prefixes []string) string {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return p
		}
	}
	return ""
}

// regexAllowed reports whether a / after prev starts a regex rather than a
// division.
func regexAllowed(prev string) bool {
	switch prev {
	case "", "(", ",", "=", ":", "[", "!", "&", "|", "?", "{", "}", ";", "+", "-", "*", "%", "<", ">", "~", "^",
		"return", "typeof", "case", "do", "else", "in", "of", "new", "delete", "void", "throw", "instanceof", "yield", "await":
		return true
	default:
		return false
	}
}

// jsxAllowed reports whether a < after prev starts an element rather than a
// comparison or a type argument.
func jsxAllowed(prev string) bool {
	switch prev {
	case "", "(", ",", "=", "?", ":", "[", "{", "&", "|", ">", "return", "yield", "default":
		return true
	default:
		return false
	}
}

// scanRegex finds the end of a regex literal starting at line[i] == '/'. It
// returns the index of the closing slash and the index after the flags.
func scanRegex(line string, i int) (int, int, bool) {
	inClass := false
	for j := i + 1; j < len(line); j++ {
		switch line[j] {
		case '\\':
			j++
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '/':
			if inClass {
				continue
			}
			if j == i+1 {
				return 0, 0, false
			}
			end := j + 1
			for end < len(line) && isLetter(line[end]) {
				end++
			}
			return j, end, true
		default:
		}
	}
	return 0, 0, false
}
