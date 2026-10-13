// Package codemask turns a unified diff into something that can be sent to a
// third-party model.
//
// The structure of the code stays: keywords, identifiers, operators, YAML keys,
// SQL statements, shell command names and flags. Everything that could carry
// customer data is replaced with a placeholder: string literals, comments, long
// numbers, JSX text, YAML values, shell arguments and heredocs.
//
// It does not try to recognise PII. A pattern can catch an email address but
// never a bare name, so codemask removes the places PII lives instead. When it
// cannot mask a file with certainty it sends only the file's path and line
// counts. It never guesses.
package codemask

import (
	"strings"
)

// Modes say how much of a file is in the result.
const (
	// ModeMasked means the file's changed lines are included, masked.
	ModeMasked = "masked"
	// ModeCounts means only the path and line counts are included.
	ModeCounts = "counts"
	// ModeBinary means the file is binary. Only the path is included.
	ModeBinary = "binary"
)

// Reasons say why a file was reduced to counts.
const (
	ReasonUnsupported = "unsupported-language" // no lexer for this file type
	ReasonData        = "data-file"            // fixtures, testdata, seeds, CSVs
	ReasonLockfile    = "lockfile"             // go.sum, package-lock.json and friends
	ReasonGenerated   = "generated"            // .pb.go, zz_generated, minified JS
	ReasonProse       = "prose"                // markdown and plain text are all content
	ReasonPartial     = "partial-context"      // the diff does not start at the top of the file
	ReasonLexer       = "lexer-state"          // the lexer ended somewhere it should not have
	ReasonPII         = "pii-pattern"          // something PII-shaped survived masking
)

// Options tune the output. The zero value is usable.
type Options struct {
	// Context is how many unchanged lines to keep around each change.
	// 0 means the default of 3. Use -1 for none.
	Context int
	// MaxLines caps the masked lines emitted across all files. Files past the
	// cap keep their counts and are marked truncated.
	// 0 means the default of 400. Use -1 for no cap.
	MaxLines int
}

// Result is a masked diff.
type Result struct {
	Files   []File  `json:"files"`
	Summary Summary `json:"summary"`
}

// Summary totals a Result.
type Summary struct {
	Files      int  `json:"files"`
	Added      int  `json:"added"`
	Removed    int  `json:"removed"`
	Masked     int  `json:"masked"`
	CountsOnly int  `json:"countsOnly"`
	Truncated  bool `json:"truncated,omitempty"`
}

// File is one file in a masked diff.
type File struct {
	Path      string   `json:"path"`
	OldPath   string   `json:"oldPath,omitempty"`
	Status    string   `json:"status"` // added, deleted, modified, renamed
	Language  string   `json:"language,omitempty"`
	Mode      string   `json:"mode"`
	Reason    string   `json:"reason,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Added     int      `json:"added"`
	Removed   int      `json:"removed"`
	Facts     *Facts   `json:"facts,omitempty"`
	Hunks     []Hunk   `json:"hunks,omitempty"`
	Truncated bool     `json:"truncated,omitempty"`
}

// Facts describe what kind of change a file has, worked out locally from the
// unmasked text. Only the booleans leave the machine.
//
// A copy change is literals changed with code unchanged. A comment-only change
// is comments changed with the other two unchanged. All three false on a file
// with changed lines means only whitespace or formatting moved.
type Facts struct {
	CodeChanged     bool `json:"codeChanged"`
	LiteralsChanged bool `json:"literalsChanged"`
	CommentsChanged bool `json:"commentsChanged"`
}

// Hunk is a run of masked lines, each prefixed with ' ', '-' or '+'.
type Hunk struct {
	Header string   `json:"header"`
	Lines  []string `json:"lines"`
}

const (
	defaultContext  = 3
	defaultMaxLines = 400
)

// Mask parses a unified diff and masks it.
//
// Masking is exact only when the lexer sees each file from its first line, so
// give it a diff with full-file context (git diff -U1000000, or the CLI's
// `codemask git`). Files in a partial-context diff are reduced to counts unless
// their language has no multi-line constructs.
func Mask(diff []byte, opts Options) (*Result, error) {
	files, err := parseDiff(diff)
	if err != nil {
		return nil, err
	}
	ctx := opts.Context
	switch {
	case ctx == 0:
		ctx = defaultContext
	case ctx < 0:
		ctx = 0
	}
	budget := opts.MaxLines
	if budget == 0 {
		budget = defaultMaxLines
	}

	res := &Result{Files: make([]File, 0, len(files))}
	for _, fd := range files {
		f := maskFile(fd, ctx, &budget)
		res.Summary.Files++
		res.Summary.Added += f.Added
		res.Summary.Removed += f.Removed
		if f.Mode == ModeMasked {
			res.Summary.Masked++
		} else {
			res.Summary.CountsOnly++
		}
		if f.Truncated {
			res.Summary.Truncated = true
		}
		res.Files = append(res.Files, f)
	}
	return res, nil
}

func maskFile(fd *fileDiff, ctx int, budget *int) File {
	path := fd.path()
	f := File{
		Path:   redactPath(path),
		Status: fd.status(),
		Tags:   pathTags(path),
	}
	if fd.status() == "renamed" {
		f.OldPath = redactPath(fd.oldPath)
	}
	for _, h := range fd.hunks {
		for _, l := range h.lines {
			switch l.op {
			case '+':
				f.Added++
			case '-':
				f.Removed++
			}
		}
	}
	if fd.binary {
		f.Mode = ModeBinary
		return f
	}

	lang := detectLanguage(path)
	reason := countsReason(path, f.Tags)
	if reason == "" && lang == nil {
		reason = ReasonUnsupported
	}
	if reason != "" {
		f.Mode, f.Reason = ModeCounts, reason
		return f
	}
	f.Language = lang.name

	lines, facts, reason := lexFile(fd, lang)
	if reason != "" {
		f.Mode, f.Reason = ModeCounts, reason
		return f
	}
	f.Facts = facts
	hunks := buildHunks(lines, ctx)
	for _, h := range hunks {
		for _, l := range h.Lines {
			if piiPattern.MatchString(l) {
				f.Mode, f.Reason = ModeCounts, ReasonPII
				return f
			}
		}
	}

	f.Mode = ModeMasked
	for _, h := range hunks {
		if *budget >= 0 && len(h.Lines) > *budget {
			f.Truncated = true
			*budget = 0
			break
		}
		if *budget >= 0 {
			*budget -= len(h.Lines)
		}
		f.Hunks = append(f.Hunks, h)
	}
	return f
}

// outLine is one lexed diff line with the positions needed to rebuild headers.
type outLine struct {
	op     byte
	text   string
	oldPos int // this line's old number, or the next old line for a '+'
	newPos int // this line's new number, or the next new line for a '-'
}

// sideFacts collects the raw pieces of one side's changed lines.
type sideFacts struct {
	skeleton strings.Builder
	literals []string
	comments []string
}

func (s *sideFacts) add(l lexedLine) {
	for _, seg := range l.segs {
		switch seg.kind {
		case segCode:
			for _, w := range strings.Fields(seg.text) {
				s.skeleton.WriteString(w)
				s.skeleton.WriteByte(' ')
			}
		case segLiteral:
			s.skeleton.WriteString("\x00L ")
			s.literals = append(s.literals, seg.text)
		case segComment:
			s.comments = append(s.comments, seg.text)
		default:
		}
	}
}

func lexFile(fd *fileDiff, lang *language) ([]outLine, *Facts, string) {
	oldL, newL := lang.newLexer(), lang.newLexer()
	expOld, expNew := 1, 1
	var lines []outLine
	var oldF, newF sideFacts

	for _, h := range fd.hunks {
		if !contiguous(h.oldStart, h.oldCount, expOld) || !contiguous(h.newStart, h.newCount, expNew) {
			if !lang.stateless {
				return nil, nil, ReasonPartial
			}
			oldL, newL = lang.newLexer(), lang.newLexer()
		}
		oldNo, newNo := h.oldStart, h.newStart
		if h.oldCount == 0 {
			oldNo++
		}
		if h.newCount == 0 {
			newNo++
		}
		for _, dl := range h.lines {
			ol := outLine{op: dl.op, oldPos: oldNo, newPos: newNo}
			switch dl.op {
			case '-':
				o := oldL.lex(dl.text)
				oldF.add(o)
				ol.text = o.masked
				oldNo++
			case '+':
				n := newL.lex(dl.text)
				newF.add(n)
				ol.text = n.masked
				newNo++
			default:
				o, n := oldL.lex(dl.text), newL.lex(dl.text)
				ol.text = moreMasked(o, n).masked
				oldNo++
				newNo++
			}
			lines = append(lines, ol)
		}
		expOld, expNew = nextLine(h.oldStart, h.oldCount), nextLine(h.newStart, h.newCount)
	}
	if !oldL.clean() || !newL.clean() {
		return nil, nil, ReasonLexer
	}
	return lines, &Facts{
		CodeChanged:     oldF.skeleton.String() != newF.skeleton.String(),
		LiteralsChanged: strings.Join(oldF.literals, "\x00") != strings.Join(newF.literals, "\x00"),
		CommentsChanged: strings.Join(oldF.comments, "\x00") != strings.Join(newF.comments, "\x00"),
	}, ""
}

// contiguous reports whether a hunk side picks up exactly where the last one
// stopped. A side with no lines sits after line start, so it continues from
// start+1.
func contiguous(start, count, expected int) bool {
	if count == 0 {
		return start+1 == expected
	}
	return start == expected
}

func nextLine(start, count int) int {
	if count == 0 {
		return start + 1
	}
	return start + count
}

// moreMasked picks between the two lexings of a context line. They differ only
// when the line sits inside a construct on one side and not the other, and then
// the one showing less code is the safe one to send.
func moreMasked(a, b lexedLine) lexedLine {
	if a.masked == b.masked || codeChars(b) <= codeChars(a) {
		return b
	}
	return a
}

func codeChars(l lexedLine) int {
	n := 0
	for _, s := range l.segs {
		if s.kind == segCode {
			n += len(strings.TrimSpace(s.text))
		}
	}
	return n
}
