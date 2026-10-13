// Command codemask masks a unified diff so it can be sent to a third-party
// model. See the README for what is kept and what is masked.
//
//	codemask [flags] < change.diff        mask a diff from stdin
//	codemask git [flags] [git diff args]  mask `git diff`, with full-file context
//	codemask demo [name]                  show the worked examples
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/thirdfort/codemask"
	"github.com/thirdfort/codemask/examples"
)

const usage = `codemask masks a unified diff so it can be sent to a third-party model.

Usage:
  codemask [flags] < change.diff        mask a diff read from stdin
  codemask git [flags] [git diff args]  run git diff with full-file context and mask it
  codemask demo [name]                  show the worked examples (all of them, or one)

Flags:
  -format json|text   output format (default json)
  -context N          unchanged lines kept around each change (default 3, -1 for none)
  -max-lines N        cap on masked lines across all files (default 400, -1 for none)

Examples:
  git diff -U1000000 main | codemask
  codemask git main...HEAD
  codemask git -format text --cached
  codemask demo 06-raw-string
`

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "codemask:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	sub := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub, args = args[0], args[1:]
	}
	switch sub {
	case "":
		return maskCmd(args, stdout, func([]string) ([]byte, error) { return io.ReadAll(stdin) })
	case "git":
		return maskCmd(args, stdout, gitDiff)
	case "demo":
		return demo(args, stdout)
	case "help":
		_, err := io.WriteString(stdout, usage)
		return err
	default:
		return fmt.Errorf("unknown command %q (try codemask help)", sub)
	}
}

func maskCmd(args []string, stdout io.Writer, input func([]string) ([]byte, error)) error {
	fs := flag.NewFlagSet("codemask", flag.ContinueOnError)
	fs.Usage = func() { _, _ = io.WriteString(fs.Output(), usage) }
	format := fs.String("format", "json", "json or text")
	ctx := fs.Int("context", 0, "context lines")
	maxLines := fs.Int("max-lines", 0, "line cap")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	diff, err := input(fs.Args())
	if err != nil {
		return err
	}
	res, err := codemask.Mask(diff, codemask.Options{Context: *ctx, MaxLines: *maxLines})
	if err != nil {
		return err
	}
	switch *format {
	case "json":
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	case "text":
		_, err := io.WriteString(stdout, res.Text())
		return err
	default:
		return fmt.Errorf("unknown format %q", *format)
	}
}

// gitDiff runs git diff with enough context to cover every file, which is what
// lets codemask lex each file from its first line.
func gitDiff(args []string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{
		"-c", "core.quotepath=off", "diff", "--no-color", "--no-ext-diff", "-U1000000",
	}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func demo(args []string, stdout io.Writer) error {
	exs := examples.All
	if len(args) > 0 {
		exs = nil
		for _, ex := range examples.All {
			if ex.Name == args[0] || strings.HasPrefix(ex.Name, args[0]+"-") {
				exs = append(exs, ex)
			}
		}
		if len(exs) == 0 {
			names := make([]string, 0, len(examples.All))
			for _, ex := range examples.All {
				names = append(names, ex.Name)
			}
			return fmt.Errorf("no example %q; there are:\n  %s", args[0], strings.Join(names, "\n  "))
		}
	}
	c := newColours(stdout)
	var sb strings.Builder
	for i, ex := range exs {
		in, err := ex.Diff()
		if err != nil {
			return err
		}
		res, err := codemask.Mask(in, codemask.Options{})
		if err != nil {
			return err
		}
		if i > 0 {
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, "%s\n%s\n\n", c.bold("━━ "+ex.Name+": "+ex.Title), wrap(ex.Description, 78))
		sb.WriteString(c.bold("The diff:") + "\n")
		c.diff(&sb, trimContext(string(in), 3))
		sb.WriteString(c.bold("What codemask sends:") + "\n")
		c.diff(&sb, res.Text())
	}
	_, err := io.WriteString(stdout, sb.String())
	return err
}

// trimContext shortens a full-context diff for display, keeping n lines either
// side of each change. Hunk headers are left as they are.
func trimContext(diff string, n int) string {
	lines := strings.Split(strings.TrimSuffix(diff, "\n"), "\n")
	keep := make([]bool, len(lines))
	inHunk := false
	for i, l := range lines {
		switch {
		case strings.HasPrefix(l, "@@"):
			inHunk = true
			keep[i] = true
		case !inHunk || strings.HasPrefix(l, "diff --git"):
			inHunk = false
			keep[i] = true
		case strings.HasPrefix(l, "+") || strings.HasPrefix(l, "-"):
			for j := max(0, i-n); j <= min(len(lines)-1, i+n); j++ {
				keep[j] = true
			}
		}
	}
	var out []string
	for i, l := range lines {
		switch {
		case keep[i]:
			out = append(out, l)
		case i > 0 && keep[i-1]:
			out = append(out, "  ⋮")
		}
	}
	return strings.Join(out, "\n") + "\n"
}

func wrap(s string, width int) string {
	var lines []string
	line := ""
	for _, w := range strings.Fields(s) {
		if line != "" && len(line)+1+len(w) > width {
			lines = append(lines, line)
			line = ""
		}
		if line != "" {
			line += " "
		}
		line += w
	}
	return strings.Join(append(lines, line), "\n")
}

type colours struct{ on bool }

func newColours(w io.Writer) colours {
	f, ok := w.(*os.File)
	if !ok || os.Getenv("NO_COLOR") != "" {
		return colours{}
	}
	st, err := f.Stat()
	return colours{on: err == nil && st.Mode()&os.ModeCharDevice != 0}
}

func (c colours) wrap(code, s string) string {
	if !c.on {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func (c colours) bold(s string) string { return c.wrap("1", s) }

func (c colours) diff(sb *strings.Builder, text string) {
	for _, l := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		switch {
		case strings.HasPrefix(l, "+++") || strings.HasPrefix(l, "---") || strings.HasPrefix(l, "diff --git") ||
			strings.HasPrefix(l, "index ") || strings.HasPrefix(l, "new file"):
			l = c.wrap("2", l)
		case strings.HasPrefix(l, "#"):
			l = c.wrap("36", l)
		case strings.HasPrefix(l, "@@"):
			l = c.wrap("35", l)
		case strings.HasPrefix(l, "+"):
			l = c.wrap("32", l)
		case strings.HasPrefix(l, "-"):
			l = c.wrap("31", l)
		}
		sb.WriteString("  " + l + "\n")
	}
	sb.WriteString("\n")
}
