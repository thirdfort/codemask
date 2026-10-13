# codemask

codemask masks a code diff so you can send it to a third-party model without sending customer
data.

It keeps the code's structure: keywords, identifiers, operators, YAML keys, SQL statements,
shell commands and flags. It replaces everything that could hold customer data with a
placeholder: string literals, comments, long numbers, JSX text, YAML values, shell arguments and
heredocs.

It doesn't try to spot PII. A pattern can catch an email address but never a bare name, so
codemask removes the places PII lives instead. If it can't mask a file with certainty, it sends
only the path and line counts.

## A quick look

A test gains a customer:

```diff
+		{
+			name: "subject with a bank account resolves",
+			subject: Subject{
+				Name:     "Jane Okafor",
+				Email:    "jane.okafor@example.com",
+				DOB:      "1987-04-12",
+				Postcode: "SW1A 1AA",
+				SortCode: 401234,
+				Account:  41000032,
+			},
+			want: StatusResolved,
+		},
```

What codemask sends:

```diff
# masked go · code changed, literals changed, comments unchanged · tags: test
+		{
+			name: "<str>",
+			subject: Subject{
+				Name:     "<str>",
+				Email:    "<str>",
+				DOB:      "<str>",
+				Postcode: "<str>",
+				SortCode: <num>,
+				Account:  <num>,
+			},
+			want: StatusResolved,
+		},
```

A model can still see this is a test adding a case with personal and bank details. It can't
see whose.

To see all twelve worked examples on your own machine:

```bash
codemask demo            # all of them
codemask demo 06         # one
```

## Install

The repo is private, so Go needs to know not to use the public proxy for it:

```bash
go env -w GOPRIVATE=github.com/thirdfort/*
go install github.com/thirdfort/codemask/cmd/codemask@latest
```

In CI, set the same `GOPRIVATE` and give git a token that can read `thirdfort/codemask`.

## Use it

### From the command line

```bash
codemask git main...HEAD                 # mask the branch's changes, as JSON
codemask git -format text --cached       # mask what's staged, for a person to read
git diff -U1000000 main | codemask       # mask a diff you made yourself
```

`codemask git` passes its arguments to `git diff` and adds `-U1000000`, which puts each whole
file in the diff. That's what makes masking exact: see
[Why full context matters](#why-full-context-matters).

| Flag | Default | Meaning |
|------|---------|---------|
| `-format` | `json` | `json` for programs, `text` for people |
| `-context` | `3` | Unchanged lines kept around each change; `-1` for none |
| `-max-lines` | `400` | Cap on masked lines across all files; `-1` for none |

### From Go

```go
res, err := codemask.Mask(diff, codemask.Options{})
if err != nil {
    return err
}
payload, _ := json.Marshal(res)   // or res.Text() for a prompt
```

## Use cases

**Classifying a pull request cheaply.** purr asks Jev what kind of PR it's looking at (copy,
tooling, UI, infra and so on) before deciding how hard to review it. The paths and tags alone
are often enough. When they aren't, the masked hunks and the facts settle it:

```bash
codemask git origin/main...HEAD | jq '[.files[] | {path, tags, facts}]'
```

A file with `codeChanged: false, literalsChanged: true` is a copy change. A file tagged
`migration` is never a copy change, however small.

**A second opinion from a model that isn't cleared for customer data.** Paste the text output
into the prompt. The model sees what the code does and how it changed, which is most of what a
review needs.

```bash
codemask git -format text main...HEAD | pbcopy
```

**Drafting a PR description or release note** with a model whose data terms you haven't checked.
Same command, different prompt.

**Checking what would leave before you send it.** Run `codemask git -format text` and read it.
If something you didn't expect is visible, that's a bug: [add an example](#adding-an-example)
that reproduces it.

**Inside a service.** Anything in blue-skies that sends code to a model can import the package
and mask first. Use the `Facts` and `Tags` on their own when the model only needs to know what
kind of change it is.

## What's kept and what's masked

| Language | Kept | Masked |
|----------|------|--------|
| Go, TypeScript, JavaScript, Python, proto, CSS/SCSS | Keywords, identifiers, operators, numbers under 6 digits | Strings, comments, numbers of 6+ digits, regex literals |
| TSX/JSX | The above, plus element and attribute names, `{expressions}` | Text between tags, attribute strings |
| SQL | Statements, table and column names | Strings, quoted identifiers, `$$` bodies, comments, long numbers |
| Terraform/HCL | Block types and labels (`resource "google_secret_manager_secret" "x"`), attributes, references | String values, heredocs, comments |
| YAML | Keys, indentation, dashes, booleans, nulls, small numbers, versions (`v1.4.2`), quantities (`512Mi`) | Every other value, block scalars, keys with spaces, comments |
| Shell, Dockerfile | Command names, flags, keywords, operators, variable names | Arguments, flag values, strings, heredocs, `${…}` expressions, comments |
| JSON | Punctuation | Every string |
| go.mod | Module paths and versions | Comments |

Delimiters stay, so `"<str>"` still reads as a string and `// <comment>` as a comment.

## When only counts are sent

| Reason | When |
|--------|------|
| `data-file` | Under `testdata/`, `fixtures/`, `seed/`, `__snapshots__/`, or a CSV, snapshot or golden file |
| `lockfile` | `go.sum`, `package-lock.json`, `bun.lock` and the like |
| `generated` | `.pb.go`, `zz_generated`, `.min.js` and the like |
| `prose` | Markdown, reStructuredText, plain text |
| `unsupported-language` | No lexer for the file type yet |
| `partial-context` | The diff doesn't start at the top of the file, and the language has constructs that span lines |
| `lexer-state` | The lexer ended inside a string or element, or hit a string that never closed. Its view of the file is wrong somewhere, so none of the file is sent |
| `pii-pattern` | Something shaped like an email, phone number, postcode, date, NI number, card number or long digit run survived masking |

The last check runs on everything that would leave, paths included. A path segment that matches
is replaced with `<redacted>`.

## Why full context matters

A hunk is a fragment of a file. If it starts inside a multi-line string, nothing in the hunk
says so. Example 06 has a YAML fixture in a Go raw string, and the change is in the middle of it:

```diff
   kind: individual
-  name: Test Subject
+  name: Jane Okafor
```

Lexed from the hunk, `name: Jane Okafor` looks like code. Lexed from the top of the file,
codemask knows it's inside a string:

```diff
 <str>
-<str>
+<str>
```

So codemask only masks a file when it has seen it from line 1. Given a normal 3-line-context
diff (example 07), it sends counts instead. JSON and go.mod have nothing that spans lines, so
they're masked from any hunk.

## Facts and tags

Each masked file carries three facts. codemask works them out locally by comparing the old and
new versions of the changed lines, and only the booleans leave:

| Fact | Meaning |
|------|---------|
| `codeChanged` | The code's structure changed, ignoring literals, comments and whitespace |
| `literalsChanged` | A string, number, value or piece of text changed |
| `commentsChanged` | A comment changed |

Only literals changed means a copy change. Only comments changed means a docs change. All three
false means only formatting moved.

Tags come from the path alone: `test`, `data`, `manifest`, `lockfile`, `generated`,
`migration`, `ci`, `iac`, `docs`, `styles`, `ui`.

## Output

```json
{
  "files": [
    {
      "path": "portal/src/components/SaveButton.tsx",
      "status": "modified",
      "language": "tsx",
      "mode": "masked",
      "tags": ["ui"],
      "added": 2,
      "removed": 2,
      "facts": { "codeChanged": false, "literalsChanged": true, "commentsChanged": false },
      "hunks": [
        {
          "header": "@@ -8,9 +8,9 @@",
          "lines": [
            " ",
            " export function SaveButton({ saving, onSave }: SaveButtonProps) {",
            "   return (",
            "-    <Button aria-label=\"<str>\" disabled={saving} onClick={onSave}>",
            "+    <Button aria-label=\"<str>\" disabled={saving} onClick={onSave}>",
            "       {saving ? <Spinner /> : null}",
            "-      <text>",
            "+      <text>",
            "     </Button>",
            "   );",
            " }"
          ]
        }
      ]
    }
  ],
  "summary": { "files": 1, "added": 2, "removed": 2, "masked": 1, "countsOnly": 0 }
}
```

`mode` is `masked`, `counts` or `binary`. A `counts` file has a `reason` and no hunks. Hunk
headers are rebuilt by codemask. The input's own section headings (`@@ … @@ func Foo`) are
dropped, because they're source lines that were never lexed.

## The examples

Each example is a before and after tree under `examples/testdata/`, the diff made from them,
and the expected output in `.golden.txt` (for reading) and `.golden.json` (for the tests).

| Example | Shows |
|---------|-------|
| [01-copy-change](examples/01-copy-change.golden.txt) | A button label change in TSX, detected as copy-only |
| [02-test-fixture](examples/02-test-fixture.golden.txt) | A Go test case full of personal and bank details |
| [03-comment-with-pii](examples/03-comment-with-pii.golden.txt) | An email and phone number in a comment |
| [04-sql-migration](examples/04-sql-migration.golden.txt) | DDL kept, backfill values masked |
| [05-terraform](examples/05-terraform.golden.txt) | Resource types kept, values masked |
| [06-raw-string](examples/06-raw-string.golden.txt) | Unquoted data inside a multi-line string |
| [07-partial-context](examples/07-partial-context.golden.txt) | The same change without full context, sent as counts |
| [08-python-docstring](examples/08-python-docstring.golden.txt) | A docstring example record |
| [09-helm-values](examples/09-helm-values.golden.txt) | Version and replica bumps visible, values masked |
| [10-shell-script](examples/10-shell-script.golden.txt) | Unquoted arguments and a heredoc |
| [11-data-file](examples/11-data-file.golden.txt) | A testdata file, sent as counts |
| [12-dependency-bump](examples/12-dependency-bump.golden.txt) | go.mod readable, go.sum as counts |

The tests also check that none of the planted names, emails, numbers or addresses appear in any
output.

## Limits

- **Identifiers are sent.** A variable called `janeOkafor` goes as it is. Customer data almost
  never ends up in identifiers, but it can.
- **Paths are sent**, apart from segments that match the PII patterns.
- **Shell is approximate.** Shell has no clean line between code and data, so codemask masks
  every argument. A misread costs signal, not data.
- **One placeholder per line.** A string that spans lines becomes one `<str>` per line, so line
  counts are visible.
- **Languages not in the table are sent as counts.** Adding one is usually a `langConfig`.

## Contributing

### Adding an example

1. Make `examples/testdata/NN-name/a/` and `b/` with the files before and after. Add `context`
   holding a number to diff with less than full context, or `from` holding another example's
   name to reuse its trees.
2. Run `examples/gen.sh` to build `NN-name.diff`.
3. Add it to `examples.All` in `examples/examples.go`.
4. Run `go test ./... -update`, then read the new `.golden.txt` before committing it.
5. If you planted new fake PII, add it to `planted` in `examples/examples_test.go`.

### Adding a language

Most languages are a `langConfig` in `lang.go`: comment markers, string delimiters and whether
strings can span lines. Map the file extension in `languages`, add cases to `TestMaskLanguages`,
and add an example if the language has anything unusual.

Before merging, run:

```bash
go test ./...
golangci-lint run ./...
```
