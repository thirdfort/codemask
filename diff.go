package codemask

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const devNull = "/dev/null"

type fileDiff struct {
	oldPath, newPath string
	binary           bool
	hunks            []rawHunk
}

type rawHunk struct {
	oldStart, oldCount int
	newStart, newCount int
	lines              []diffLine
}

type diffLine struct {
	op   byte // ' ', '-' or '+'
	text string
}

func (fd *fileDiff) path() string {
	if fd.newPath == "" || fd.newPath == devNull {
		return fd.oldPath
	}
	return fd.newPath
}

func (fd *fileDiff) status() string {
	switch {
	case fd.oldPath == devNull:
		return "added"
	case fd.newPath == devNull:
		return "deleted"
	case fd.oldPath != "" && fd.newPath != "" && fd.oldPath != fd.newPath:
		return "renamed"
	default:
		return "modified"
	}
}

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// parseDiff reads a unified diff, with or without git's extended headers.
// Hunk bodies are read by their declared line counts, never by sniffing, since a
// removed SQL comment ("-- note") looks exactly like a file header ("--- a/x").
func parseDiff(data []byte) ([]*fileDiff, error) {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	var files []*fileDiff
	var cur *fileDiff
	start := func() *fileDiff {
		cur = &fileDiff{}
		files = append(files, cur)
		return cur
	}

	for i := 0; i < len(lines); i++ {
		l := lines[i]
		switch {
		case strings.HasPrefix(l, "diff --git "):
			start()
			cur.oldPath, cur.newPath = gitHeaderPaths(strings.TrimPrefix(l, "diff --git "))
		case strings.HasPrefix(l, "--- ") && i+1 < len(lines) && strings.HasPrefix(lines[i+1], "+++ "):
			if cur == nil || len(cur.hunks) > 0 {
				start()
			}
			cur.oldPath = headerPath(strings.TrimPrefix(l, "--- "), "a/")
			cur.newPath = headerPath(strings.TrimPrefix(lines[i+1], "+++ "), "b/")
			i++
		case strings.HasPrefix(l, "@@ "):
			if cur == nil {
				return nil, fmt.Errorf("codemask: line %d: hunk before any file header", i+1)
			}
			h, next, err := readHunk(lines, i)
			if err != nil {
				return nil, err
			}
			cur.hunks = append(cur.hunks, h)
			i = next - 1
		case cur == nil:
		case strings.HasPrefix(l, "Binary files ") || l == "GIT binary patch":
			cur.binary = true
		case strings.HasPrefix(l, "new file mode"):
			cur.oldPath = devNull
		case strings.HasPrefix(l, "deleted file mode"):
			cur.newPath = devNull
		case strings.HasPrefix(l, "rename from "):
			cur.oldPath = strings.TrimPrefix(l, "rename from ")
		case strings.HasPrefix(l, "rename to "):
			cur.newPath = strings.TrimPrefix(l, "rename to ")
		default:
		}
	}
	return files, nil
}

// readHunk reads the hunk whose header is at lines[i] and returns the index of
// the line after it.
func readHunk(lines []string, i int) (rawHunk, int, error) {
	m := hunkHeader.FindStringSubmatch(lines[i])
	if m == nil {
		return rawHunk{}, 0, fmt.Errorf("codemask: line %d: bad hunk header %q", i+1, lines[i])
	}
	h := rawHunk{
		oldStart: atoi(m[1]), oldCount: countOr1(m[2]),
		newStart: atoi(m[3]), newCount: countOr1(m[4]),
	}
	oldLeft, newLeft := h.oldCount, h.newCount
	j := i + 1
	for ; (oldLeft > 0 || newLeft > 0) && j < len(lines); j++ {
		l := lines[j]
		if strings.HasPrefix(l, `\`) {
			continue
		}
		op, text := byte(' '), ""
		if l != "" {
			op, text = l[0], l[1:]
		}
		switch op {
		case ' ':
			oldLeft--
			newLeft--
		case '-':
			oldLeft--
		case '+':
			newLeft--
		default:
			return rawHunk{}, 0, fmt.Errorf("codemask: line %d: unexpected %q inside a hunk", j+1, l)
		}
		h.lines = append(h.lines, diffLine{op: op, text: strings.TrimSuffix(text, "\r")})
	}
	if oldLeft > 0 || newLeft > 0 {
		return rawHunk{}, 0, fmt.Errorf("codemask: line %d: hunk ends early", j)
	}
	for j < len(lines) && strings.HasPrefix(lines[j], `\`) {
		j++
	}
	return h, j, nil
}

func headerPath(s, prefix string) string {
	if tab := strings.IndexByte(s, '\t'); tab >= 0 {
		s = s[:tab]
	}
	if s == devNull {
		return s
	}
	return strings.TrimPrefix(s, prefix)
}

// gitHeaderPaths splits "a/x b/x". The paths may contain spaces, so it splits
// at the last " b/".
func gitHeaderPaths(s string) (string, string) {
	idx := strings.LastIndex(s, " b/")
	if idx < 0 || !strings.HasPrefix(s, "a/") {
		return "", ""
	}
	return s[2:idx], s[idx+3:]
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func countOr1(s string) int {
	if s == "" {
		return 1
	}
	return atoi(s)
}

// buildHunks keeps the changed lines and ctx lines either side of them, and
// writes fresh headers. The input's own section headings are dropped: they are
// source lines (a function signature, or a variable holding a string) that were
// never lexed.
func buildHunks(lines []outLine, ctx int) []Hunk {
	keep := make([]bool, len(lines))
	for i, l := range lines {
		if l.op == ' ' {
			continue
		}
		for j := max(0, i-ctx); j <= min(len(lines)-1, i+ctx); j++ {
			keep[j] = true
		}
	}
	var hunks []Hunk
	for i := 0; i < len(lines); {
		if !keep[i] {
			i++
			continue
		}
		j := i
		for j < len(lines) && keep[j] {
			j++
		}
		hunks = append(hunks, makeHunk(lines[i:j]))
		i = j
	}
	return hunks
}

func makeHunk(lines []outLine) Hunk {
	oldCount, newCount := 0, 0
	out := make([]string, len(lines))
	for i, l := range lines {
		if l.op != '+' {
			oldCount++
		}
		if l.op != '-' {
			newCount++
		}
		out[i] = string(l.op) + l.text
	}
	oldStart, newStart := lines[0].oldPos, lines[0].newPos
	if oldCount == 0 {
		oldStart--
	}
	if newCount == 0 {
		newStart--
	}
	return Hunk{
		Header: fmt.Sprintf("@@ -%d,%d +%d,%d @@", oldStart, oldCount, newStart, newCount),
		Lines:  out,
	}
}
