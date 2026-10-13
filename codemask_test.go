package codemask

import (
	"fmt"
	"strings"
	"testing"
)

// addedFile builds a diff that adds path with content, so the whole file is
// lexed from its first line.
func addedFile(path, content string) []byte {
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	var b strings.Builder
	fmt.Fprintf(&b, "--- /dev/null\n+++ b/%s\n@@ -0,0 +1,%d @@\n", path, len(lines))
	for _, l := range lines {
		b.WriteString("+" + l + "\n")
	}
	return []byte(b.String())
}

func maskOne(t *testing.T, diff []byte) File {
	t.Helper()
	res, err := Mask(diff, Options{Context: -1, MaxLines: -1})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Files) != 1 {
		t.Fatalf("got %d files, want 1", len(res.Files))
	}
	return res.Files[0]
}

func maskedText(f *File) string {
	var out []string
	for _, h := range f.Hunks {
		for _, l := range h.Lines {
			out = append(out, l[1:])
		}
	}
	return strings.Join(out, "\n")
}

func TestMaskLanguages(t *testing.T) {
	tests := []struct {
		name, path, in, want string
	}{
		{
			name: "template literal with nested strings and templates",
			path: "a.ts",
			in:   "const s = `hi ${user.name + \"x\"} and ${`inner ${a}`}`;\nconst t = 1;",
			want: "const s = `<str>`;\nconst t = 1;",
		},
		{
			name: "regex containing quotes",
			path: "a.js",
			in:   "const re = /\"[a-z]+'/g;\nconst x = \"jane\";",
			want: "const re = /<re>/g;\nconst x = \"<str>\";",
		},
		{
			name: "division is not a regex",
			path: "a.js",
			in:   "const r = a / b / c;",
			want: "const r = a / b / c;",
		},
		{
			name: "type arguments in .ts are not JSX",
			path: "a.ts",
			in:   "const m = new Map<string, number>();\nconst ok = a < b && c > d;",
			want: "const m = new Map<string, number>();\nconst ok = a < b && c > d;",
		},
		{
			name: "JSX text, nesting, fragments and expressions",
			path: "a.tsx",
			in: `const el = (
  <div className="card">
    Hello {user.name}, welcome
    <b>Jane</b>
    <>Okafor</>
    {items.map((i) => <li key={i.id}>{i.label} item</li>)}
  </div>
);
const n = a < b;`,
			want: `const el = (
  <div className="<str>">
    <text> {user.name}<text>
    <b><text></b>
    <><text></>
    {items.map((i) => <li key={i.id}>{i.label} <text></li>)}
  </div>
);
const n = a < b;`,
		},
		{
			name: "python f-strings, comments and triple quotes",
			path: "a.py",
			in:   "x = f\"hi {name}\"  # Jane\ny = '''\nJane\n'''",
			want: "x = f\"<str>\"  # <comment>\ny = '''\n<str>\n'''",
		},
		{
			name: "SQL doubled quotes and dollar quotes",
			path: "q.sql",
			in:   "SELECT 'it''s Jane', $$ body 'x' $$, $fn$ y $fn$, $1 FROM t; -- Jane",
			want: "SELECT '<str>', $$<str>$$, $fn$<str>$fn$, $1 FROM t; -- <comment>",
		},
		{
			name: "HCL heredoc",
			path: "main.tf",
			in:   "locals {\n  policy = <<EOF\n{\"owner\": \"jane\"}\nEOF\n}",
			want: "locals {\n  policy = <<EOF\n<str>\nEOF\n}",
		},
		{
			name: "HCL labels only on block lines, and only identifier-shaped",
			path: "main.tf",
			in:   "module \"jane okafor\" {\n  name = \"subjects\"\n}",
			want: "module \"<str>\" {\n  name = \"<str>\"\n}",
		},
		{
			name: "long numbers are masked, short ones kept",
			path: "a.go",
			in:   "port := 8080; acct := 12345678; ver := 1.2; hex := 0xFFFFFF",
			want: "port := 8080; acct := <num>; ver := 1.2; hex := <num>",
		},
		{
			name: "comment markers inside strings",
			path: "a.go",
			in:   "s := \"http://example.com/jane\" /* note: Jane\nOkafor */ + t",
			want: "s := \"<str>\" /*<comment>\n<comment>*/ + t",
		},
		{
			name: "shell keeps commands and flags, masks arguments",
			path: "x.sh",
			in:   "FOO=bar baz --flag=x -v \"q\" 'r' $HOME ${X:-def} | grep -i jane > out.txt",
			want: "FOO=<arg> baz --flag=<arg> -v \"<str>\" '<str>' $HOME ${<expr>} | grep -i <arg> > <arg>",
		},
		{
			name: "shell control flow",
			path: "x.sh",
			in:   "for f in a b; do\n  if test -f \"$f\"; then rm -- \"$f\"; fi\ndone",
			want: "for f in <arg> <arg>; do\n  if test -f \"<str>\"; then rm -- \"<str>\"; fi\ndone",
		},
		{
			name: "dockerfile",
			path: "Dockerfile",
			in:   "FROM golang:1.22\nRUN apt-get install -y curl\nENV OWNER=jane",
			want: "FROM <arg>\nRUN apt-get <arg> -y <arg>\nENV <arg>",
		},
		{
			name: "yaml keys with spaces, flow values, multi-line quotes",
			path: "c.yaml",
			in:   "Jane Okafor: approved\nmeta: {owner: jane}\nnote: \"first line\n  second line\"\nok: true",
			want: "<key>: <val>\nmeta: <val>\nnote: \"<str>\n<str>\"\nok: true",
		},
		{
			name: "yaml sequences and block scalars",
			path: "c.yml",
			in:   "steps:\n  - run: |\n      echo jane\n      echo done\n  - name: build\nafter: 1",
			want: "steps:\n  - run: |\n      <str>\n      <str>\n  - name: <val>\nafter: 1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := maskOne(t, addedFile(tt.path, tt.in))
			if f.Mode != ModeMasked {
				t.Fatalf("mode %s (%s), want masked", f.Mode, f.Reason)
			}
			if got := maskedText(&f); got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestCountsOnly(t *testing.T) {
	tests := []struct {
		name, path, in, reason string
	}{
		{"unterminated string", "a.go", "x := \"abc\ny := 1", ReasonLexer},
		{"string still open at the end", "a.go", "x := `abc", ReasonLexer},
		{"JSX left open", "a.tsx", "const x = <div>", ReasonLexer},
		{"PII-shaped code survives masking", "a.go", "x := jane@example.com", ReasonPII},
		{"unsupported language", "a.rb", "puts 'hi'", ReasonUnsupported},
		{"markdown", "README.md", "# Hi", ReasonProse},
		{"testdata", "pkg/testdata/x.go", "package x", ReasonData},
		{"generated", "api/v1/check.pb.go", "package v1", ReasonGenerated},
		{"lockfile", "package-lock.json", "{}", ReasonLockfile},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := maskOne(t, addedFile(tt.path, tt.in))
			if f.Mode != ModeCounts || f.Reason != tt.reason {
				t.Errorf("mode %s reason %q, want counts %q", f.Mode, f.Reason, tt.reason)
			}
			if len(f.Hunks) != 0 {
				t.Errorf("counts-only file has hunks: %v", f.Hunks)
			}
		})
	}
}

func TestPartialContext(t *testing.T) {
	diff := "--- a/a.go\n+++ b/a.go\n@@ -10,3 +10,3 @@\n x\n-y := 1\n+y := 2\n z\n"
	if f := maskOne(t, []byte(diff)); f.Reason != ReasonPartial {
		t.Errorf("go: reason %q, want %q", f.Reason, ReasonPartial)
	}
	// JSON has nothing that spans lines, so any hunk can be lexed on its own.
	diff = "--- a/c.json\n+++ b/c.json\n@@ -10,2 +10,2 @@\n {\n-  \"owner\": \"jane\"\n+  \"owner\": \"tom\"\n"
	f := maskOne(t, []byte(diff))
	if f.Mode != ModeMasked {
		t.Fatalf("json: mode %s (%s), want masked", f.Mode, f.Reason)
	}
	if got, want := maskedText(&f), "  \"<str>\": \"<str>\"\n  \"<str>\": \"<str>\""; got != want {
		t.Errorf("json: got\n%s\nwant\n%s", got, want)
	}
}

func TestFacts(t *testing.T) {
	tests := []struct {
		name     string
		old, new string
		want     Facts
	}{
		{"copy", `x := "Save"`, `x := "Save changes"`, Facts{LiteralsChanged: true}},
		{"comment", "x := 1 // a", "x := 1 // b", Facts{CommentsChanged: true}},
		{"code", "x := 1", "x := 2", Facts{CodeChanged: true}},
		{"whitespace", "x := 1", "x  :=  1", Facts{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff := fmt.Sprintf("--- a/a.go\n+++ b/a.go\n@@ -1,1 +1,1 @@\n-%s\n+%s\n", tt.old, tt.new)
			f := maskOne(t, []byte(diff))
			if f.Facts == nil || *f.Facts != tt.want {
				t.Errorf("facts %+v, want %+v", f.Facts, tt.want)
			}
		})
	}
}

func TestDiffParsing(t *testing.T) {
	t.Run("a removed SQL comment is not a file header", func(t *testing.T) {
		diff := "--- a/q.sql\n+++ b/q.sql\n@@ -1,2 +1,1 @@\n--- note\n SELECT 1;\n"
		f := maskOne(t, []byte(diff))
		if f.Removed != 1 || f.Added != 0 || maskedText(&f) != "-- <comment>" {
			t.Errorf("got %+v: %q", f, maskedText(&f))
		}
	})
	t.Run("rename and binary", func(t *testing.T) {
		diff := "diff --git a/old.png b/new.png\nsimilarity index 90%\nrename from old.png\nrename to new.png\nBinary files a/old.png and b/new.png differ\n"
		f := maskOne(t, []byte(diff))
		if f.Status != "renamed" || f.OldPath != "old.png" || f.Path != "new.png" || f.Mode != ModeBinary {
			t.Errorf("got %+v", f)
		}
	})
	t.Run("short hunk is an error", func(t *testing.T) {
		if _, err := Mask([]byte("--- a/x.go\n+++ b/x.go\n@@ -1,3 +1,3 @@\n x\n"), Options{}); err == nil {
			t.Error("want an error")
		}
	})
	t.Run("section headings are dropped", func(t *testing.T) {
		diff := "--- a/a.go\n+++ b/a.go\n@@ -1,1 +1,1 @@ var owner = \"jane@example.com\"\n-x := 1\n+x := 2\n"
		f := maskOne(t, []byte(diff))
		if f.Hunks[0].Header != "@@ -1,1 +1,1 @@" {
			t.Errorf("header %q", f.Hunks[0].Header)
		}
	})
}

func TestRedactPath(t *testing.T) {
	for in, want := range map[string]string{
		"pkg/x.go":                                 "pkg/x.go",
		"testdata/jane@example.com.json":           "testdata/<redacted>.json",
		"subjects/07700900123/notes/1987-04-12.go": "subjects/<redacted>/notes/<redacted>.go",
	} {
		if got := redactPath(in); got != want {
			t.Errorf("redactPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestContextAndBudget(t *testing.T) {
	var src strings.Builder
	for i := range 20 {
		fmt.Fprintf(&src, "x%d := %d\n", i, i)
	}
	diff := addedFile("a.go", src.String())

	res, err := Mask(diff, Options{MaxLines: 5})
	if err != nil {
		t.Fatal(err)
	}
	if f := res.Files[0]; !f.Truncated || len(f.Hunks) != 0 || !res.Summary.Truncated {
		t.Errorf("budget: got %+v", f)
	}

	old := strings.Replace(src.String(), "x10 := 10", "x10 := 11", 1)
	var b strings.Builder
	b.WriteString("--- a/a.go\n+++ b/a.go\n@@ -1,20 +1,20 @@\n")
	oldLines, newLines := strings.Split(old, "\n"), strings.Split(src.String(), "\n")
	for i := range 20 {
		if oldLines[i] == newLines[i] {
			b.WriteString(" " + oldLines[i] + "\n")
			continue
		}
		b.WriteString("-" + oldLines[i] + "\n+" + newLines[i] + "\n")
	}
	res, err = Mask([]byte(b.String()), Options{Context: 2})
	if err != nil {
		t.Fatal(err)
	}
	h := res.Files[0].Hunks
	if len(h) != 1 || h[0].Header != "@@ -9,5 +9,5 @@" || len(h[0].Lines) != 6 {
		t.Errorf("context: got %+v", h)
	}
}
