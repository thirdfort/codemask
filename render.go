package codemask

import (
	"fmt"
	"strings"
)

// Text renders a Result as a diff for people to read. It is not a patch: the
// `#` lines say what codemask did with each file.
func (r *Result) Text() string {
	var sb strings.Builder
	for i := range r.Files {
		f := &r.Files[i]
		oldPath, newPath := "a/"+f.Path, "b/"+f.Path
		if f.OldPath != "" {
			oldPath = "a/" + f.OldPath
		}
		switch f.Status {
		case "added":
			oldPath = devNull
		case "deleted":
			newPath = devNull
		}
		fmt.Fprintf(&sb, "--- %s\n+++ %s\n# %s\n", oldPath, newPath, f.describe())
		for _, h := range f.Hunks {
			sb.WriteString(h.Header)
			sb.WriteByte('\n')
			for _, l := range h.Lines {
				sb.WriteString(l)
				sb.WriteByte('\n')
			}
		}
		if f.Truncated {
			sb.WriteString("# truncated: line budget reached\n")
		}
	}
	return sb.String()
}

func (f *File) describe() string {
	parts := []string{}
	switch f.Mode {
	case ModeMasked:
		parts = append(parts, "masked "+f.Language)
	case ModeBinary:
		parts = append(parts, "binary")
	default:
		parts = append(parts, fmt.Sprintf("counts only (+%d -%d): %s", f.Added, f.Removed, f.Reason))
	}
	if f.Facts != nil {
		parts = append(parts, f.Facts.String())
	}
	if len(f.Tags) > 0 {
		parts = append(parts, "tags: "+strings.Join(f.Tags, ","))
	}
	return strings.Join(parts, " · ")
}

// String says what changed in words: "code unchanged, literals changed".
func (fa *Facts) String() string {
	word := func(changed bool, what string) string {
		if changed {
			return what + " changed"
		}
		return what + " unchanged"
	}
	return strings.Join([]string{
		word(fa.CodeChanged, "code"),
		word(fa.LiteralsChanged, "literals"),
		word(fa.CommentsChanged, "comments"),
	}, ", ")
}
