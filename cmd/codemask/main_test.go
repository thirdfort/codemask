package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/thirdfort/codemask"
)

func TestStdin(t *testing.T) {
	in := "--- a/a.go\n+++ b/a.go\n@@ -1,1 +1,1 @@\n-x := \"jane\"\n+x := \"tom\"\n"
	var out strings.Builder
	if err := run(nil, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	var res codemask.Result
	if err := json.Unmarshal([]byte(out.String()), &res); err != nil {
		t.Fatal(err)
	}
	if got := res.Files[0].Hunks[0].Lines; strings.Join(got, "|") != `-x := "<str>"|+x := "<str>"` {
		t.Errorf("got %q", got)
	}
}

func TestDemo(t *testing.T) {
	var out strings.Builder
	if err := run([]string{"demo"}, nil, &out); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(out.String(), "What codemask sends:"); n != 12 {
		t.Errorf("demo showed %d examples, want 12", n)
	}
	if err := run([]string{"demo", "nope"}, nil, &out); err == nil {
		t.Error("want an error for an unknown example")
	}
}
