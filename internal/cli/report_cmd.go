package cli

import (
	"context"
	"fmt"

	"github.com/0xbenc/dangit/internal/report"
	"github.com/0xbenc/dangit/internal/scan"
	"github.com/0xbenc/dangit/internal/ui"
)

// runDefault handles the bare `dangit [PATH]` invocation: interactive browser on
// a TTY, plain report otherwise.
func (r runner) runDefault(args []string) int {
	f, positional, err := parseFlags(args)
	if err != nil {
		return r.usageErr(err)
	}
	if f.help {
		fmt.Fprint(r.stdout, usage)
		return 0
	}
	path, err := singlePath(positional)
	if err != nil {
		return r.usageErr(err)
	}
	if err := validateDir(path); err != nil {
		return r.usageErr(err)
	}

	interactive := r.stdoutTTY && r.stdinTTY && !f.plain && !f.json
	if interactive {
		return r.runInteractive(path, f)
	}
	return r.runReport(path, f)
}

// runScan is the explicit non-interactive report command.
func (r runner) runScan(args []string) int {
	f, positional, err := parseFlags(args)
	if err != nil {
		return r.usageErr(err)
	}
	if f.help {
		fmt.Fprint(r.stdout, usage)
		return 0
	}
	path, err := singlePath(positional)
	if err != nil {
		return r.usageErr(err)
	}
	if err := validateDir(path); err != nil {
		return r.usageErr(err)
	}
	return r.runReport(path, f)
}

// runReport scans path and writes a text or JSON report. Exit code is 1 when
// any repo needs attention.
func (r runner) runReport(path string, f flags) int {
	timeout, err := r.resolveTimeout(f)
	if err != nil {
		return r.usageErr(err)
	}
	results, err := scan.Scan(context.Background(), scan.Options{
		Root:      path,
		Timeout:   timeout,
		NoNetwork: r.resolveNoNetwork(f),
		Env:       r.env,
	})
	if err != nil {
		fmt.Fprintf(r.stderr, "dangit: %v\n", err)
		return 1
	}

	label := report.RootLabel(path)
	if f.json {
		if err := report.JSON(r.stdout, results, report.Options{Root: label}); err != nil {
			fmt.Fprintf(r.stderr, "dangit: %v\n", err)
			return 1
		}
	} else {
		theme := r.resolveTheme(f, r.stdoutTTY)
		report.Text(r.stdout, results, report.Options{Root: label, Theme: theme})
	}

	if scan.Summarize(results).Flagged > 0 {
		return 1
	}
	return 0
}

// runInteractive launches the bubbletea browser.
func (r runner) runInteractive(path string, f flags) int {
	timeout, err := r.resolveTimeout(f)
	if err != nil {
		return r.usageErr(err)
	}
	remaining, err := ui.Browse(context.Background(), ui.BrowseOptions{
		Root:        path,
		RootLabel:   report.RootLabel(path),
		Timeout:     timeout,
		NoNetwork:   r.resolveNoNetwork(f),
		NoColor:     f.noColor,
		NoAltScreen: f.noAltScreen,
		Theme:       r.resolveTheme(f, !f.noColor),
		Env:         r.env,
	})
	if err != nil {
		fmt.Fprintf(r.stderr, "dangit: %v\n", err)
		return 1
	}
	if remaining > 0 {
		return 1
	}
	return 0
}
