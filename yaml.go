package codemask

import (
	"regexp"
	"strings"
)

// yamlLexer keeps the shape of a YAML document (indentation, keys, sequence
// dashes, block indicators) and masks every value, except booleans, nulls and
// small numbers, versions and k8s quantities, which carry structure rather
// than data (`replicas: 3`, `tag: v1.4.2`, `memory: 512Mi`).
type yamlLexer struct {
	block int  // the owner's indent while inside a block scalar, else -1
	quote byte // a quoted scalar open across lines
}

func newYAML() *yamlLexer { return &yamlLexer{block: -1} }

func (y *yamlLexer) clean() bool { return y.quote == 0 }

var (
	yamlPlainKey = regexp.MustCompile(`^[A-Za-z0-9_.\-/$@{}<>]+$`)
	yamlKeep     = regexp.MustCompile(`(?i)^(true|false|null|~|yes|no|on|off|[-+]?[0-9]{1,5}(\.[0-9]+)?|[&*][A-Za-z0-9_-]+|\{\}|\[\]|v?[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?|[0-9]+(\.[0-9]+)?(Ki|Mi|Gi|Ti|m|k|M|G))$`)
	yamlBlock    = regexp.MustCompile(`^[|>][-+0-9]*`)
)

func (y *yamlLexer) lex(line string) lexedLine {
	var b lineBuilder
	if y.quote != 0 {
		y.continueQuote(&b, line)
		return b.done()
	}
	trimmed := strings.TrimLeft(line, " ")
	indent := len(line) - len(trimmed)
	if y.block >= 0 {
		if strings.TrimSpace(line) == "" || indent > y.block {
			b.code(line[:indent])
			if t := strings.TrimSpace(trimmed); t != "" {
				b.mask(segLiteral, phString, t)
			}
			return b.done()
		}
		y.block = -1
	}
	b.code(line[:indent])
	switch {
	case trimmed == "":
		return b.done()
	case trimmed[0] == '#':
		yamlComment(&b, trimmed)
		return b.done()
	case trimmed == "---" || trimmed == "...":
		b.code(trimmed)
		return b.done()
	}

	col := indent
	for trimmed == "-" || strings.HasPrefix(trimmed, "- ") {
		dash := trimmed[:1] + leadingSpace(trimmed[1:])
		b.code(dash)
		col += len(dash)
		trimmed = trimmed[len(dash):]
	}
	if key, after, ok := yamlSplitKey(trimmed); ok {
		yamlKey(&b, key)
		b.code(":")
		y.value(&b, after, col)
	} else {
		y.value(&b, trimmed, col)
	}
	return b.done()
}

// yamlSplitKey finds "key: value" or "key:". A plain key may not contain ": ".
func yamlSplitKey(s string) (string, string, bool) {
	if s == "" || strings.ContainsRune("{[&*!|>%@`", rune(s[0])) {
		return "", "", false
	}
	if s[0] == '"' || s[0] == '\'' {
		end := strings.IndexByte(s[1:], s[0])
		if end < 0 {
			return "", "", false
		}
		end += 2
		if end < len(s) && s[end] == ':' && (end+1 == len(s) || isSpace(s[end+1])) {
			return s[:end], s[end+1:], true
		}
		return "", "", false
	}
	idx := strings.Index(s, ": ")
	if strings.HasSuffix(s, ":") && (idx < 0 || idx == len(s)-1) {
		idx = len(s) - 1
	}
	if idx <= 0 || strings.Contains(s[:idx], " #") {
		return "", "", false
	}
	return s[:idx], s[idx+1:], true
}

// yamlKey keeps keys that read as identifiers. A key with spaces, or a quoted
// one that is not identifier-like, is masked: in a map keyed by data, the keys
// are the data.
func yamlKey(b *lineBuilder, key string) {
	inner := key
	quoted := key[0] == '"' || key[0] == '\''
	if quoted {
		inner = key[1 : len(key)-1]
	}
	if yamlPlainKey.MatchString(inner) {
		b.code(key)
		return
	}
	if quoted {
		b.code(key[:1])
		b.mask(segLiteral, phKey, inner)
		b.code(key[:1])
		return
	}
	b.mask(segLiteral, phKey, key)
}

func (y *yamlLexer) value(b *lineBuilder, v string, col int) {
	ws := leadingSpace(v)
	b.code(ws)
	v = v[len(ws):]
	switch {
	case v == "":
		return
	case v[0] == '#':
		yamlComment(b, v)
		return
	case v[0] == '|' || v[0] == '>':
		ind := yamlBlock.FindString(v)
		b.code(ind)
		y.block = col
		y.tail(b, v[len(ind):])
		return
	case v[0] == '"' || v[0] == '\'':
		q := v[0]
		b.code(v[:1])
		end := yamlQuoteEnd(v[1:], q)
		if end < 0 {
			if v[1:] != "" {
				b.mask(segLiteral, phString, v[1:])
			}
			y.quote = q
			return
		}
		if end > 0 {
			b.mask(segLiteral, phString, v[1:1+end])
		}
		b.code(v[1+end : 2+end])
		y.tail(b, v[2+end:])
		return
	}
	val, comment := v, ""
	if k := strings.Index(v, " #"); k >= 0 {
		val, comment = v[:k], v[k:]
	}
	t := strings.TrimRight(val, " \t")
	if yamlKeep.MatchString(t) {
		b.code(val)
	} else {
		b.mask(segLiteral, phValue, t)
		b.code(trailingSpace(val))
	}
	y.tail(b, comment)
}

// tail handles what follows a value on its line: nothing, or a comment.
func (y *yamlLexer) tail(b *lineBuilder, s string) {
	ws := leadingSpace(s)
	b.code(ws)
	s = s[len(ws):]
	switch {
	case s == "":
	case s[0] == '#':
		yamlComment(b, s)
	default:
		b.mask(segLiteral, phValue, s)
	}
}

func (y *yamlLexer) continueQuote(b *lineBuilder, line string) {
	end := yamlQuoteEnd(line, y.quote)
	if end < 0 {
		if line != "" {
			b.mask(segLiteral, phString, line)
		}
		return
	}
	if end > 0 {
		b.mask(segLiteral, phString, line[:end])
	}
	b.code(line[end : end+1])
	y.quote = 0
	y.tail(b, line[end+1:])
}

// yamlQuoteEnd finds the closing quote: \" escapes in double quotes, ” in single.
func yamlQuoteEnd(s string, q byte) int {
	for i := 0; i < len(s); i++ {
		switch {
		case q == '"' && s[i] == '\\':
			i++
		case s[i] == q && q == '\'' && i+1 < len(s) && s[i+1] == '\'':
			i++
		case s[i] == q:
			return i
		}
	}
	return -1
}

func yamlComment(b *lineBuilder, s string) {
	b.delim("#")
	body := s[1:]
	ws := leadingSpace(body)
	b.delim(ws)
	if t := strings.TrimRight(body[len(ws):], " \t"); t != "" {
		b.mask(segComment, phComment, t)
	}
}
