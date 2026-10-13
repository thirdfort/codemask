// Package examples holds codemask's worked examples: real-shaped diffs with fake
// customer data in them, and what codemask makes of each. They are embedded so
// `codemask demo` can show them from an installed binary, and the tests check
// every one against its expected output.
package examples

import (
	"embed"
)

//go:embed *.diff
var diffs embed.FS

// Example is one worked example.
type Example struct {
	Name        string // the file stem: 01-copy-change
	Title       string
	Description string
}

// Diff returns the example's input.
func (e Example) Diff() ([]byte, error) {
	return diffs.ReadFile(e.Name + ".diff")
}

// All is every example, in order.
var All = []Example{
	{
		Name:  "01-copy-change",
		Title: "A copy change in a React component",
		Description: "A button label changes, in an attribute and in the element's text. JSX text is " +
			"masked like a string. The facts say code unchanged, literals changed: a copy change, " +
			"without anyone reading the copy.",
	},
	{
		Name:  "02-test-fixture",
		Title: "A Go test with a customer in it",
		Description: "A new test case with a name, email, date of birth, postcode, sort code and " +
			"account number. Every one is a literal, so every one goes. The struct's field names, " +
			"which say what kind of data the test uses, stay.",
	},
	{
		Name:  "03-comment-with-pii",
		Title: "A comment that names the person who reported a bug",
		Description: "Only a comment changes, and the comment has an email address and phone number " +
			"in it. The comment is masked. The facts still say exactly what happened: comments " +
			"changed, nothing else did.",
	},
	{
		Name:  "04-sql-migration",
		Title: "A migration with a backfill",
		Description: "A new migration adds a column and backfills it for one subject. The DDL reads " +
			"in full; the values in the WHERE clause, including an 8-digit account number, are " +
			"masked. The path is tagged as a migration.",
	},
	{
		Name:  "05-terraform",
		Title: "A new Terraform resource",
		Description: "A secret is added. Resource types, block structure and references (var.project_id) " +
			"stay. String values, including one with ${…} interpolation, are masked.",
	},
	{
		Name:  "06-raw-string",
		Title: "Customer data inside a multi-line string",
		Description: "A Go raw string holds a YAML fixture, and the change edits lines in the middle " +
			"of it. Those lines have no quotes of their own, so a lexer that started at the hunk " +
			"would read `name: Jane Okafor` as code. With the whole file in the diff, codemask " +
			"knows they are inside a string, and masks them.",
	},
	{
		Name:  "07-partial-context",
		Title: "The same change, without the whole file",
		Description: "The same change as 06, diffed with git's default 3 lines of context. The hunk " +
			"starts inside the string, and nothing in the diff says so. codemask cannot be sure, so " +
			"it sends the path and line counts only. This is what failing closed looks like; " +
			"`codemask git` always diffs with full context to avoid it.",
	},
	{
		Name:  "08-python-docstring",
		Title: "A docstring with an example record",
		Description: "A docstring's example row gains a name and email, and a field is added to the " +
			"code. Triple-quoted strings are masked across lines. The code change still shows.",
	},
	{
		Name:  "09-helm-values",
		Title: "A Helm values bump",
		Description: "An image tag bump, a replica count, a feature flag and a free-text note. Keys " +
			"stay. Values are masked, except booleans, nulls and small numbers, which are structure " +
			"rather than data. The note is a block scalar, so every line of it is masked.",
	},
	{
		Name:  "10-shell-script",
		Title: "A shell script with a hard-coded subject",
		Description: "In shell the data is mostly unquoted, so codemask masks bare arguments as well as " +
			"strings. Command names, flags, keywords, operators and variable names stay, which is " +
			"usually enough to tell what a script does. Heredoc bodies are masked.",
	},
	{
		Name:  "11-data-file",
		Title: "A testdata file",
		Description: "Anything under testdata/, fixtures/ or seed/, and CSV or snapshot files, is data " +
			"by definition. Only the path and line counts go.",
	},
	{
		Name:  "12-dependency-bump",
		Title: "A dependency bump",
		Description: "go.mod is masked like code, so module paths and versions read in full. go.sum is " +
			"a lockfile and goes as counts. Both paths are tagged, so a consumer can tell a " +
			"dependency bump from the tags alone.",
	},
}
