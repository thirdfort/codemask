package codemask

import (
	"regexp"
	"slices"
	"strings"
)

// shell masks shell scripts and Dockerfiles. Unlike C-family code, a shell
// command's data is mostly unquoted (`psql -c "…" jane@example.com`), so bare
// arguments are masked too. What stays is the shape of each command: its name,
// its flags, keywords, operators and variable names.
type shell struct {
	docker   bool
	cmdPos   bool // the next word is a command name
	first    bool // dockerfile: the next word is an instruction
	nameNext bool // the next word is a name: for x, case x, function x
	cont     bool // the last line ended with a backslash
	quote    byte // a quoted string open across lines
	docs     []heredoc
	inDoc    *heredoc
	dirty    bool
	b        lineBuilder
	p        pending
}

type heredoc struct {
	marker    string
	stripTabs bool
}

func newShell(docker bool) *shell { return &shell{docker: docker} }

func (s *shell) clean() bool {
	return !s.dirty && s.quote == 0 && s.inDoc == nil && len(s.docs) == 0
}

// Keywords and prefix commands after which a command name comes next.
var shellCmdNext = []string{
	"if", "then", "else", "elif", "while", "until", "do", "!", "time", "{",
	"exec", "sudo", "env", "command", "nohup", "xargs",
}

// Builtins whose arguments are assignments, so FOO=bar after them stays readable.
var shellDeclare = []string{"export", "local", "declare", "readonly", "typeset"}

var (
	shellAssign  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\[[^\]]*\])?\+?=`)
	shellName    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	shellHeredoc = regexp.MustCompile(`^<<(-?)\s*(['"]?)\\?([A-Za-z_][A-Za-z0-9_]*)['"]?`)
	shellVarName = regexp.MustCompile(`^\$\{[A-Za-z_][A-Za-z0-9_]*\}`)
	shellSimple  = regexp.MustCompile(`^\$([A-Za-z_][A-Za-z0-9_]*|[0-9@*#?$!-])`)
	smallNumber  = regexp.MustCompile(`^\d{1,5}$`)
)

// Operators, longest first. Each one either starts a new command or does not.
var shellOps = []struct {
	op     string
	newCmd bool
}{
	{"2>&1", false},
	{"$((", true},
	{"&&", true},
	{"||", true},
	{";;", true},
	{"|&", true},
	{"$(", true},
	{"<(", true},
	{">(", true},
	{">>", false},
	{"&>", false},
	{">&", false},
	{"<<<", false},
	{"|", true},
	{"&", true},
	{";", true},
	{"(", true},
	{")", false},
	{"<", false},
	{">", false},
}

func (s *shell) lex(line string) lexedLine {
	s.b = lineBuilder{}
	if s.inDoc != nil {
		s.heredocLine(line)
		return s.b.done()
	}
	if !s.cont {
		s.cmdPos, s.first = true, s.docker
	}
	s.cont = false
	i := 0
	if s.quote != 0 {
		i = s.quoted(line, 0)
	}
	for i < len(line) {
		i = s.step(line, i)
	}
	s.p.flush(&s.b)
	if s.inDoc == nil && len(s.docs) > 0 {
		s.inDoc, s.docs = &s.docs[0], s.docs[1:]
	}
	return s.b.done()
}

func (s *shell) heredocLine(line string) {
	check := line
	if s.inDoc.stripTabs {
		check = strings.TrimLeft(line, "\t")
	}
	if check == s.inDoc.marker {
		s.b.code(line)
		s.inDoc = nil
		if len(s.docs) > 0 {
			s.inDoc, s.docs = &s.docs[0], s.docs[1:]
		}
		return
	}
	if line != "" {
		s.b.mask(segLiteral, phString, line)
	}
}

func (s *shell) step(line string, i int) int {
	c, rest := line[i], line[i:]
	switch {
	case isSpace(c):
		j := i
		for j < len(line) && isSpace(line[j]) {
			j++
		}
		s.b.code(line[i:j])
		return j
	case c == '#' && (i == 0 || isSpace(line[i-1]) || strings.ContainsRune(";|&(", rune(line[i-1]))):
		s.b.delim("#")
		if body := strings.TrimSpace(rest[1:]); body != "" {
			s.b.delim(leadingSpace(rest[1:]))
			s.b.mask(segComment, phComment, body)
		}
		return len(line)
	case c == '\'' || c == '"' || c == '`':
		s.b.code(rest[:1])
		s.quote = c
		s.p.start(segLiteral, phString)
		s.cmdPos, s.first = false, false
		return s.quoted(line, i+1)
	case c == '\\' && i == len(line)-1:
		s.b.code(`\`)
		s.cont = true
		return len(line)
	case strings.HasPrefix(rest, "<<") && !strings.HasPrefix(rest, "<<<"):
		if m := shellHeredoc.FindStringSubmatch(rest); m != nil {
			s.b.code(m[0])
			s.docs = append(s.docs, heredoc{marker: m[3], stripTabs: m[1] == "-"})
			return i + len(m[0])
		}
	case c == '$' && !strings.HasPrefix(rest, "$(") && !strings.HasPrefix(rest, "$'"):
		return s.variable(line, i)
	}
	for _, o := range shellOps {
		if strings.HasPrefix(rest, o.op) {
			s.b.code(o.op)
			s.cmdPos = o.newCmd
			return i + len(o.op)
		}
	}
	if c == '$' {
		s.b.code("$")
		return i + 1
	}
	return s.word(line, i)
}

func (s *shell) quoted(line string, i int) int {
	j := i
	for j < len(line) {
		c := line[j]
		if c == '\\' && s.quote != '\'' {
			j += 2
			continue
		}
		if c == s.quote {
			s.p.add(line[i:j])
			s.p.flush(&s.b)
			s.b.code(line[j : j+1])
			s.quote = 0
			return j + 1
		}
		j++
	}
	s.p.add(line[i:min(j, len(line))])
	return len(line)
}

func (s *shell) variable(line string, i int) int {
	rest := line[i:]
	if m := shellVarName.FindString(rest); m != "" {
		s.b.code(m)
		s.cmdPos = false
		return i + len(m)
	}
	if strings.HasPrefix(rest, "${") {
		end := expansionEnd(rest)
		if end < 0 {
			s.dirty = true
			s.b.mask(segLiteral, phExpr, rest)
			return len(line)
		}
		s.b.code("${")
		s.b.mask(segLiteral, phExpr, rest[2:end])
		s.b.code("}")
		s.cmdPos = false
		return i + end + 1
	}
	if m := shellSimple.FindString(rest); m != "" {
		s.b.code(m)
		s.cmdPos = false
		return i + len(m)
	}
	s.b.code("$")
	return i + 1
}

// expansionEnd returns the index of the } that closes the ${ at the start of
// rest, or -1. Quotes and nested ${…} inside it are skipped over, as in
// ${flags[@]+"${flags[@]}"}.
func expansionEnd(rest string) int {
	depth := 0
	for j := 0; j < len(rest); j++ {
		switch c := rest[j]; {
		case c == '\\':
			j++
		case c == '\'' || c == '"':
			k := j + 1
			for k < len(rest) && rest[k] != c {
				if rest[k] == '\\' && c == '"' {
					k++
				}
				k++
			}
			j = k
		case strings.HasPrefix(rest[j:], "${"):
			depth++
			j++
		case c == '}':
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return -1
}

// word reads one unquoted word and decides what it is. A word stops at a
// quote or a $, so --name="…" and --name=$NAME split into a flag and a value.
func (s *shell) word(line string, i int) int {
	j := i
	for j < len(line) {
		c := line[j]
		if isSpace(c) || strings.IndexByte("|&;()<>'\"`$", c) >= 0 {
			break
		}
		if c == '\\' {
			if j+1 == len(line) {
				break
			}
			j++
		}
		j++
	}
	if j == i {
		s.b.code(line[i : i+1])
		return i + 1
	}
	s.classify(line[i:j])
	return j
}

func (s *shell) classify(w string) {
	switch {
	case s.nameNext:
		s.nameNext = false
		if shellName.MatchString(w) {
			s.b.code(w)
		} else {
			s.b.mask(segLiteral, phArg, w)
		}
	case s.first:
		s.first = false
		s.b.code(w)
		s.cmdPos = strings.EqualFold(w, "RUN")
	case s.cmdPos:
		s.command(w)
	case strings.HasPrefix(w, "-") && len(w) > 1 && (isLetter(w[1]) || w[1] == '-'):
		if eq := strings.IndexByte(w, '='); eq >= 0 {
			s.b.code(w[:eq+1])
			if eq+1 < len(w) {
				s.b.mask(segLiteral, phArg, w[eq+1:])
			}
			return
		}
		s.b.code(w)
	case w == "--" || w == "in" || w == "]" || w == "]]" || smallNumber.MatchString(w):
		s.b.code(w)
	default:
		s.b.mask(segLiteral, phArg, w)
	}
}

func (s *shell) command(w string) {
	if m := shellAssign.FindString(w); m != "" {
		s.b.code(m)
		if len(m) < len(w) {
			s.b.mask(segLiteral, phArg, w[len(m):])
		}
		return
	}
	s.b.code(w)
	switch {
	case slices.Contains(shellCmdNext, w), slices.Contains(shellDeclare, w):
		// Still at a command: `if grep …`, `export FOO=…`.
	case w == "for" || w == "select" || w == "case" || w == "function":
		s.cmdPos, s.nameNext = false, true
	default:
		s.cmdPos = false
	}
}
