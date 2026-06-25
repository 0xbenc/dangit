// Package cli is dangit's command-line front end: argument parsing, command
// dispatch, exit-code discipline, and the non-interactive report/resolve/theme
// paths. The interactive browser lives in internal/ui.
package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"

	"github.com/0xbenc/dangit/internal/termstyle"
)

const usage = `dangit — find git repos with unfinished work

Usage:
  dangit [PATH] [flags]         Browse flagged repos (TTY) or print a report
  dangit scan [PATH] [flags]    Print a report; never interactive
  dangit resolve [PATH] [flags] Commit, pull --rebase, and push flagged repos
  dangit theme import PATH      Replace the active theme with a .theme file
  dangit version                Print build version information
  dangit help                   Show this help

Flags:
  -t, --timeout-secs N   Per-repo remote-check timeout in seconds (default 10)
      --no-network       Skip remote checks (offline; behind shows as stale)
      --json             Machine-readable output (scan)
      --plain            Force a plain report even on a TTY
      --no-color         Disable colored output
      --no-alt-screen    Render the browser inline (no alternate screen)
      --theme-file PATH  Load the theme from PATH
  -y, --yes              Actually perform resolve actions (default: dry run)
  -m, --message MSG      Commit message for resolve (default: generated)

Environment:
  DANGIT_TIMEOUT_SECS  Default per-repo timeout
  DANGIT_NO_NETWORK    Skip remote checks when truthy
  DANGIT_NO_COLOR      Disable color when truthy
  DANGIT_THEME_FILE    Theme config path
  NO_COLOR             Disable color when set

Exit codes: 0 clean · 1 repos need attention · 2 usage error
`

const themeUsage = `Usage:
  dangit theme import PATH

Replace the active theme with a portable .theme file written by any
termtheme-based app (passage, ssherpa, dangit). The previous theme.conf is
backed up. Roles dangit does not paint are preserved for the next app.
`

// BuildInfo carries link-time version metadata.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

func (b BuildInfo) normalized() BuildInfo {
	return BuildInfo{
		Version: defaultString(b.Version, "dev"),
		Commit:  defaultString(b.Commit, "none"),
		Date:    defaultString(b.Date, "unknown"),
	}
}

// Run is the process entry point. It returns the exit code.
func Run(args []string, stdout io.Writer, stderr io.Writer, build BuildInfo) int {
	stdout = writerOrDiscard(stdout)
	stderr = writerOrDiscard(stderr)
	r := runner{
		stdout:    stdout,
		stderr:    stderr,
		env:       os.Environ(),
		build:     build.normalized(),
		stdoutTTY: isFileTTY(os.Stdout),
		stdinTTY:  isFileTTY(os.Stdin),
	}
	return r.run(args)
}

type runner struct {
	stdout    io.Writer
	stderr    io.Writer
	env       []string
	build     BuildInfo
	stdoutTTY bool
	stdinTTY  bool
}

func (r runner) run(args []string) int {
	if len(args) == 0 {
		return r.runDefault(nil)
	}
	switch args[0] {
	case "help", "--help", "-h":
		fmt.Fprint(r.stdout, usage)
		return 0
	case "version", "--version", "-v":
		return r.runVersion()
	case "scan":
		return r.runScan(args[1:])
	case "resolve":
		return r.runResolve(args[1:])
	case "theme":
		return r.runTheme(args[1:])
	default:
		return r.runDefault(args)
	}
}

func (r runner) runVersion() int {
	fmt.Fprintf(r.stdout, "dangit %s (commit %s, built %s)\n", r.build.Version, r.build.Commit, r.build.Date)
	return 0
}

// usageErr reports a validation error to stderr and returns exit code 2.
func (r runner) usageErr(err error) int {
	fmt.Fprintf(r.stderr, "dangit: %v\n", err)
	fmt.Fprintln(r.stderr, `Run "dangit help" for usage.`)
	return 2
}

// resolveTheme builds the render theme, disabling color when not allowed.
func (r runner) resolveTheme(f flags, colorAllowed bool) termstyle.Theme {
	theme, err := termstyle.ResolveTheme(termstyle.ThemeOptions{
		File:    f.themeFile,
		NoColor: f.noColor || !colorAllowed,
		Env:     r.env,
	})
	if err != nil {
		return termstyle.TerminalTheme().WithNoColor(f.noColor || !colorAllowed)
	}
	return theme
}

func (r runner) envValue(key string) string {
	value := ""
	for _, item := range r.env {
		if k, v, ok := strings.Cut(item, "="); ok && k == key {
			value = v
		}
	}
	return value
}

func (r runner) resolveNoNetwork(f flags) bool {
	return f.noNetwork || envTruthy(r.envValue("DANGIT_NO_NETWORK"))
}

func isFileTTY(f *os.File) bool {
	return f != nil && term.IsTerminal(f.Fd())
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func writerOrDiscard(w io.Writer) io.Writer {
	if w == nil {
		return io.Discard
	}
	return w
}

func envTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "0", "false", "no", "off":
		return false
	default:
		return true
	}
}
