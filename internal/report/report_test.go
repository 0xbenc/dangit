package report_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/0xbenc/dangit/internal/report"
	"github.com/0xbenc/dangit/internal/scan"
	"github.com/0xbenc/dangit/internal/termstyle"
)

func plainTheme() termstyle.Theme {
	return termstyle.TerminalTheme().WithNoColor(true)
}

func TestTextClean(t *testing.T) {
	var b bytes.Buffer
	results := []scan.Result{{Path: "repo", Branch: "main", Ahead: "0", Behind: "0"}}
	n := report.Text(&b, results, report.Options{Root: "x", Theme: plainTheme()})
	if n != 0 {
		t.Fatalf("flagged = %d, want 0", n)
	}
	if !strings.Contains(b.String(), "No repositories need attention.") {
		t.Errorf("missing clean line:\n%s", b.String())
	}
}

func TestTextFlagged(t *testing.T) {
	var b bytes.Buffer
	results := []scan.Result{
		{Path: "alpha", Branch: "main", HasChanges: true, Ahead: "0", Behind: "0"},
		{Path: "beta", Branch: "dev", Ahead: "2", Behind: scan.BehindUnknown},
		{Path: "ok", Branch: "main", Ahead: "0", Behind: "0"},
	}
	n := report.Text(&b, results, report.Options{Root: "x", Theme: plainTheme()})
	if n != 2 {
		t.Fatalf("flagged = %d, want 2", n)
	}
	out := b.String()
	for _, want := range []string{
		"Needs attention (2):",
		"alpha",
		"changes",
		"beta",
		"ahead (2)",
		"behind (unknown)",
		"changes: 1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	// The non-flagged "ok" repo must not appear in the list.
	if strings.Contains(out, "(main)  ok") {
		t.Errorf("non-flagged repo leaked into report:\n%s", out)
	}
}

func TestJSONFlaggedOnly(t *testing.T) {
	var b bytes.Buffer
	results := []scan.Result{
		{Path: "alpha", AbsPath: "/x/alpha", Branch: "main", HasChanges: true},
		{Path: "ok", AbsPath: "/x/ok", Branch: "main", Ahead: "0", Behind: "0"},
	}
	if err := report.JSON(&b, results, report.Options{Root: "x"}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, `"alpha"`) {
		t.Errorf("expected alpha in JSON:\n%s", out)
	}
	if strings.Contains(out, `"ok"`) {
		t.Errorf("non-flagged repo should be omitted from JSON repos:\n%s", out)
	}
}
