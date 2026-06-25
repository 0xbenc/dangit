package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/0xbenc/dangit/internal/scan"
	"github.com/0xbenc/dangit/internal/termstyle"
)

// runResolve commits, rebase-pulls, and pushes flagged repos under PATH. It is a
// dry run (prints the plan) unless --yes is given, and refuses when the network
// is disabled.
func (r runner) runResolve(args []string) int {
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
	if r.resolveNoNetwork(f) {
		fmt.Fprintln(r.stderr, "dangit: resolve needs network access; unset --no-network / DANGIT_NO_NETWORK")
		return 2
	}

	timeout, err := r.resolveTimeout(f)
	if err != nil {
		return r.usageErr(err)
	}
	ctx := context.Background()
	results, err := scan.Scan(ctx, scan.Options{Root: path, Timeout: timeout, Env: r.env})
	if err != nil {
		fmt.Fprintf(r.stderr, "dangit: %v\n", err)
		return 1
	}
	flagged := scan.Flagged(results)
	theme := r.resolveTheme(f, r.stdoutTTY)

	if len(flagged) == 0 {
		fmt.Fprintln(r.stdout, theme.Style(termstyle.RoleSuccess, "Nothing to resolve."))
		return 0
	}

	if !f.yes {
		return r.resolveDryRun(ctx, flagged, f, theme)
	}
	return r.resolveExecute(ctx, flagged, f, timeout, theme)
}

func (r runner) resolveDryRun(ctx context.Context, flagged []scan.Result, f flags, theme termstyle.Theme) int {
	fmt.Fprintln(r.stdout, theme.Style(termstyle.RoleTitle,
		fmt.Sprintf("Dry run — %d repo(s) would be resolved (pass --yes to execute):", len(flagged))))
	for _, res := range flagged {
		plan, err := scan.PlanResolve(ctx, res.AbsPath, f.message, r.env)
		if err != nil {
			fmt.Fprintf(r.stdout, "  %s %s — %s\n",
				theme.Style(termstyle.RoleDanger, "✗"), res.Path,
				theme.Style(termstyle.RoleMuted, err.Error()))
			continue
		}
		fmt.Fprintf(r.stdout, "  %s %s  %s\n",
			theme.Style(termstyle.RoleAccent, "•"), res.Path, planSummary(theme, plan))
	}
	return 1
}

func (r runner) resolveExecute(ctx context.Context, flagged []scan.Result, f flags, timeout time.Duration, theme termstyle.Theme) int {
	failures := 0
	for _, res := range flagged {
		out := scan.Resolve(ctx, res.AbsPath, scan.ResolveOptions{
			Env:     r.env,
			Timeout: timeout,
			Message: f.message,
		})
		if out.Err != nil {
			failures++
			fmt.Fprintf(r.stdout, "  %s %s — %s\n",
				theme.Style(termstyle.RoleDanger, "✗"), res.Path,
				theme.Style(termstyle.RoleDanger, out.Err.Error()))
			continue
		}
		fmt.Fprintf(r.stdout, "  %s %s — %s\n",
			theme.Style(termstyle.RoleSuccess, "✓"), res.Path,
			theme.Style(termstyle.RoleMuted, out.Message))
	}
	if failures > 0 {
		return 1
	}
	return 0
}

// planSummary renders a one-line description of what a resolve would do.
func planSummary(theme termstyle.Theme, plan scan.ResolvePlan) string {
	if plan.Blocked != "" {
		return theme.Style(termstyle.RoleWarning, "blocked: ") + theme.Style(termstyle.RoleMuted, plan.Blocked)
	}
	var steps []string
	if plan.WillCommit {
		steps = append(steps, theme.Style(termstyle.RoleWarning, "commit")+" "+
			theme.Style(termstyle.RoleMuted, fmt.Sprintf("(%q)", plan.CommitMsg)))
	}
	if plan.WillPull {
		steps = append(steps, theme.Style(termstyle.RoleInfo, "pull --rebase"))
	}
	if plan.WillPush {
		steps = append(steps, theme.Style(termstyle.RoleSuccess, "push"))
	}
	if len(steps) == 0 {
		return theme.Style(termstyle.RoleMuted, "nothing to do")
	}
	return joinSteps(steps)
}

func joinSteps(steps []string) string {
	out := ""
	for i, s := range steps {
		if i > 0 {
			out += " → "
		}
		out += s
	}
	return out
}
