package examples_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"regexp"
	"testing"

	"github.com/thirdfort/codemask"
	"github.com/thirdfort/codemask/examples"
)

var update = flag.Bool("update", false, "rewrite the expected output")

// TestExamples checks each example against its expected output. After an
// intended change, run `go test ./... -update` and read the diff of the
// .golden files before committing them.
func TestExamples(t *testing.T) {
	for _, ex := range examples.All {
		t.Run(ex.Name, func(t *testing.T) {
			in, err := ex.Diff()
			if err != nil {
				t.Fatal(err)
			}
			res, err := codemask.Mask(in, codemask.Options{})
			if err != nil {
				t.Fatal(err)
			}
			js, err := json.MarshalIndent(res, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			js = append(js, '\n')
			golden(t, ex.Name+".golden.json", js)
			golden(t, ex.Name+".golden.txt", []byte(res.Text()))
		})
	}
}

// The fake people in the examples. None of this may appear in any output.
var planted = regexp.MustCompile(`(?i)jane|okafor|nowak|example\.com|07700|900123|1987|SW1A|41000032|401234|533401827|elm row|edinburgh`)

func TestExamplesLeakNothing(t *testing.T) {
	for _, ex := range examples.All {
		in, err := ex.Diff()
		if err != nil {
			t.Fatal(err)
		}
		res, err := codemask.Mask(in, codemask.Options{})
		if err != nil {
			t.Fatal(err)
		}
		js, err := json.Marshal(res)
		if err != nil {
			t.Fatal(err)
		}
		if m := planted.Find(js); m != nil {
			t.Errorf("%s: output contains %q", ex.Name, m)
		}
	}
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	if *update {
		if err := os.WriteFile(name, got, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("%v (run go test ./... -update to create it)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from expected:\n--- got\n%s\n--- want\n%s", name, got, want)
	}
}
