// Package ui is dangit's interactive terminal browser: a bubbletea v2 program
// that shows live scan progress and then a fuzzy-filterable list of flagged
// repositories with per-repo actions (detail, shell, editor, copy, resolve).
package ui

import (
	"context"
	"io"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/0xbenc/dangit/internal/termstyle"
)

// BrowseOptions configures the interactive browser.
type BrowseOptions struct {
	Root        string
	RootLabel   string
	Timeout     time.Duration
	NoNetwork   bool
	NoColor     bool
	NoAltScreen bool
	Theme       termstyle.Theme
	Env         []string
	Input       io.Reader
	Output      io.Writer
}

// Browse runs the scan-and-browse program and returns the number of repos that
// still need attention when the user quits (used for the process exit code).
func Browse(ctx context.Context, opts BrowseOptions) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	m := newModel(ctx, opts)

	programOptions := []tea.ProgramOption{tea.WithContext(ctx)}
	if opts.Input != nil {
		programOptions = append(programOptions, tea.WithInput(opts.Input))
	}
	if opts.Output != nil {
		programOptions = append(programOptions, tea.WithOutput(opts.Output))
	}
	final, err := tea.NewProgram(m, programOptions...).Run()
	if err != nil {
		return 0, err
	}
	if mod, ok := final.(model); ok {
		return mod.remainingFlagged(), nil
	}
	return 0, nil
}
