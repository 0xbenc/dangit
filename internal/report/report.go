// Package report renders scan results as either a human-readable, theme-aware
// text report or a stable JSON envelope. The text form mirrors the original
// forgit output: a header, an aligned "needs attention" list, and a summary.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/0xbenc/dangit/internal/scan"
	"github.com/0xbenc/dangit/internal/termstyle"
)

// Options configures rendering.
type Options struct {
	// Root is the display label for the scan root (see RootLabel).
	Root string
	// Theme styles the text output. A NoColor theme yields plain text.
	Theme termstyle.Theme
}

// RootLabel produces the header label for a scan root: "<base> (cwd)" when it
// is the current directory, otherwise the path as given.
func RootLabel(root string) string {
	if root == "" {
		root = "."
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return root
	}
	if cwd, err := os.Getwd(); err == nil && abs == cwd {
		return filepath.Base(abs) + " (cwd)"
	}
	return root
}

// Text renders the full human report to w and returns the number of flagged
// repositories.
func Text(w io.Writer, results []scan.Result, opts Options) int {
	t := opts.Theme
	flagged := scan.Flagged(results)

	fmt.Fprintln(w, t.Style(termstyle.RoleMuted,
		fmt.Sprintf("Scanning %s under: %s", pluralRepos(len(results)), opts.Root)))

	if len(flagged) == 0 {
		fmt.Fprintln(w, t.Style(termstyle.RoleSuccess, "No repositories need attention."))
		return 0
	}

	fmt.Fprintln(w, t.Style(termstyle.RoleTitle, fmt.Sprintf("Needs attention (%d):", len(flagged))))
	width := 0
	for _, r := range flagged {
		if vw := termstyle.VisibleWidth(r.Path); vw > width {
			width = vw
		}
	}
	for _, r := range flagged {
		path := termstyle.PadRight(r.Path, width)
		branch := t.Style(termstyle.RolePrimary, "("+r.Branch+")")
		fmt.Fprintf(w, "  %s %s  %s  %s\n",
			t.Style(termstyle.RoleAccent, "•"), path, branch, statusParts(t, r))
	}

	s := scan.Summarize(results)
	fmt.Fprintln(w, t.Style(termstyle.RoleMuted, "—"))
	fmt.Fprintln(w, summaryLine(t, "changes", termstyle.RoleWarning, s.Changes, ""))
	fmt.Fprintln(w, summaryLine(t, "ahead", termstyle.RoleInfo, s.Ahead,
		fmt.Sprintf("%d no-remote", s.AheadNoUpstream)))
	fmt.Fprintln(w, summaryLine(t, "behind", termstyle.RoleDanger, s.Behind,
		fmt.Sprintf("%d unknown, %d stale", s.BehindUnknown, s.BehindStale)))
	return len(flagged)
}

// statusParts renders the colored, comma-separated status descriptors for a
// flagged repo, in the order changes, ahead, behind.
func statusParts(t termstyle.Theme, r scan.Result) string {
	var parts []string
	if r.HasChanges {
		parts = append(parts, t.Style(termstyle.RoleWarning, "changes"))
	}
	if r.Ahead != "" && r.Ahead != scan.StateNone {
		qualifier := r.Ahead
		if r.Ahead == scan.AheadNoUpstream {
			qualifier = "no remote"
		}
		parts = append(parts, t.Style(termstyle.RoleInfo, "ahead")+" "+
			t.Style(termstyle.RoleMuted, "("+qualifier+")"))
	}
	if r.Behind != "" && r.Behind != scan.StateNone {
		parts = append(parts, t.Style(termstyle.RoleDanger, "behind")+" "+
			t.Style(termstyle.RoleMuted, "("+r.Behind+")"))
	}
	return strings.Join(parts, ", ")
}

func summaryLine(t termstyle.Theme, label string, role termstyle.Role, count int, extra string) string {
	line := t.Style(termstyle.RoleMuted, label+": ") + t.Style(role, strconv.Itoa(count))
	if extra != "" {
		line += " " + t.Style(termstyle.RoleMuted, "("+extra+")")
	}
	return line
}

func pluralRepos(n int) string {
	if n == 1 {
		return "1 Git repo"
	}
	return strconv.Itoa(n) + " Git repos"
}

// Envelope is the JSON output shape.
type Envelope struct {
	Root    string        `json:"root"`
	Summary scan.Summary  `json:"summary"`
	Repos   []scan.Result `json:"repos"`
}

// JSON renders the machine-readable envelope (flagged repos only).
func JSON(w io.Writer, results []scan.Result, opts Options) error {
	env := Envelope{
		Root:    opts.Root,
		Summary: scan.Summarize(results),
		Repos:   scan.Flagged(results),
	}
	if env.Repos == nil {
		env.Repos = []scan.Result{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(env)
}
