package scan

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ResolvePlan describes what Resolve will do to a repo, for preview/confirmation.
type ResolvePlan struct {
	Repo       string   // display path
	AbsPath    string   // absolute working-tree path
	Branch     string   // current branch (empty if detached)
	WillCommit bool     // uncommitted changes will be auto-committed
	CommitMsg  string   // the generated (or supplied) commit subject
	Files      []string // changed paths to be committed (status order)
	WillPull   bool     // a rebase pull will run
	WillPush   bool     // a push will run (an upstream is configured)
	Blocked    string   // non-empty when the repo cannot be fully resolved (e.g. detached, no upstream)
}

// ResolveResult is the outcome of executing a ResolvePlan.
type ResolveResult struct {
	Repo      string
	Committed bool
	Pulled    bool
	Pushed    bool
	Message   string // human-readable summary or failure reason
	Err       error
}

// ResolveOptions tunes a resolve run.
type ResolveOptions struct {
	Env     []string
	Timeout time.Duration
	// Message overrides the generated commit subject when non-empty.
	Message string
}

// PlanResolve inspects repoDir and builds a ResolvePlan without changing
// anything. customMsg, when non-empty, overrides the generated commit subject.
func PlanResolve(ctx context.Context, repoDir string, customMsg string, env []string) (ResolvePlan, error) {
	g := newGitRunner(env, DefaultTimeout, false)
	abs := repoDir
	plan := ResolvePlan{Repo: repoDir, AbsPath: abs}

	if out, err := g.git(ctx, repoDir, "rev-parse", "--is-inside-work-tree"); err != nil || strings.TrimSpace(out) != "true" {
		return plan, fmt.Errorf("%s is not a git working tree", repoDir)
	}

	branch, err := g.git(ctx, repoDir, "symbolic-ref", "--short", "-q", "HEAD")
	branch = sanitizeLine(branch)
	if err != nil || branch == "" {
		plan.Blocked = "detached HEAD — checkout a branch first"
		return plan, nil
	}
	plan.Branch = branch

	status, _ := g.git(ctx, repoDir, "status", "--porcelain")
	plan.Files = porcelainPaths(status)
	plan.WillCommit = len(plan.Files) > 0
	plan.CommitMsg = strings.TrimSpace(customMsg)
	if plan.CommitMsg == "" {
		plan.CommitMsg = generatedSubject(plan.Files)
	}

	remote, _ := g.git(ctx, repoDir, "config", "--get", "branch."+branch+".remote")
	remote = strings.TrimSpace(remote)
	if remote == "" || remote == "." {
		plan.Blocked = "no upstream remote — set one with `git push -u`"
		return plan, nil
	}
	plan.WillPull = true
	plan.WillPush = true
	return plan, nil
}

// Resolve brings a flagged repo to a clean, synced state: auto-commit any
// uncommitted changes, rebase-pull from the upstream, then push. It never
// force-pushes, never creates merge commits, and aborts (leaving any new commit
// in place) if the rebase conflicts. The caller is responsible for confirmation
// and for refusing when the network is disabled.
func Resolve(ctx context.Context, repoDir string, opts ResolveOptions) ResolveResult {
	g := newGitRunner(opts.Env, timeoutOrDefault(opts.Timeout), false)
	res := ResolveResult{Repo: repoDir}

	if out, err := g.git(ctx, repoDir, "rev-parse", "--is-inside-work-tree"); err != nil || strings.TrimSpace(out) != "true" {
		res.Err = fmt.Errorf("%s is not a git working tree", repoDir)
		return res
	}

	branch, err := g.git(ctx, repoDir, "symbolic-ref", "--short", "-q", "HEAD")
	branch = sanitizeLine(branch)
	if err != nil || branch == "" {
		res.Err = fmt.Errorf("detached HEAD — checkout a branch first")
		return res
	}

	// 1. Commit local work.
	status, _ := g.git(ctx, repoDir, "status", "--porcelain")
	files := porcelainPaths(status)
	if len(files) > 0 {
		if _, err := g.git(ctx, repoDir, "add", "-A"); err != nil {
			res.Err = fmt.Errorf("git add failed: %w", err)
			return res
		}
		subject := strings.TrimSpace(opts.Message)
		if subject == "" {
			subject = generatedSubject(files)
		}
		body := generatedBody(status)
		args := []string{"commit", "-m", subject}
		if body != "" {
			args = append(args, "-m", body)
		}
		if _, err := g.git(ctx, repoDir, args...); err != nil {
			res.Err = fmt.Errorf("git commit failed: %w", err)
			return res
		}
		res.Committed = true
	}

	// Upstream required for pull/push.
	remote, _ := g.git(ctx, repoDir, "config", "--get", "branch."+branch+".remote")
	remote = strings.TrimSpace(remote)
	if remote == "" || remote == "." {
		res.Message = "committed locally; no upstream remote, not pushed"
		return res
	}

	// 2. Integrate the remote by rebasing local commits on top. On conflict,
	// abort so the working tree is left clean (with the new commit intact).
	if _, err := g.gitNet(ctx, repoDir, "fetch"); err != nil {
		res.Err = fmt.Errorf("git fetch failed: %w", err)
		return res
	}
	if _, err := g.gitNet(ctx, repoDir, "pull", "--rebase"); err != nil {
		_, _ = g.git(ctx, repoDir, "rebase", "--abort")
		res.Err = fmt.Errorf("pull --rebase failed (conflict?) — resolve manually")
		return res
	}
	res.Pulled = true

	// 3. Push.
	if _, err := g.gitNet(ctx, repoDir, "push"); err != nil {
		res.Err = fmt.Errorf("git push failed: %w", err)
		return res
	}
	res.Pushed = true
	res.Message = resolveSummary(res)
	return res
}

func resolveSummary(res ResolveResult) string {
	parts := make([]string, 0, 3)
	if res.Committed {
		parts = append(parts, "committed")
	}
	if res.Pulled {
		parts = append(parts, "pulled")
	}
	if res.Pushed {
		parts = append(parts, "pushed")
	}
	if len(parts) == 0 {
		return "already in sync"
	}
	return strings.Join(parts, ", ")
}

// generatedSubject builds the deterministic auto-commit subject. Offline and
// template-based by design — no AI, no network.
func generatedSubject(files []string) string {
	n := len(files)
	if n == 0 {
		return "dangit: snapshot working tree"
	}
	subject := fmt.Sprintf("dangit: snapshot %d file", n)
	if n != 1 {
		subject += "s"
	}
	preview := files
	more := 0
	if len(preview) > 3 {
		more = len(preview) - 3
		preview = preview[:3]
	}
	subject += " (" + strings.Join(preview, ", ")
	if more > 0 {
		subject += fmt.Sprintf(", +%d more", more)
	}
	subject += ")"
	return subject
}

// generatedBody lists the porcelain status lines as the commit body.
func generatedBody(status string) string {
	lines := make([]string, 0)
	for _, ln := range strings.Split(status, "\n") {
		if strings.TrimSpace(ln) != "" {
			lines = append(lines, ln)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return "Changed files:\n" + strings.Join(lines, "\n")
}

// porcelainPaths extracts the changed paths from `git status --porcelain`
// output, handling rename arrows and quoted names well enough for a preview.
func porcelainPaths(status string) []string {
	var paths []string
	for _, ln := range strings.Split(status, "\n") {
		if len(ln) < 4 {
			continue
		}
		// Format: "XY <path>" or "XY <old> -> <new>".
		rest := strings.TrimSpace(ln[3:])
		if rest == "" {
			rest = strings.TrimSpace(ln)
		}
		if idx := strings.Index(rest, " -> "); idx >= 0 {
			rest = rest[idx+4:]
		}
		rest = strings.Trim(rest, "\"")
		if rest != "" {
			paths = append(paths, rest)
		}
	}
	return paths
}

// ResolveNetworkAllowed reports whether resolve is permitted given the env's
// no-network setting; resolve fundamentally needs the network. The last
// matching DANGIT_NO_NETWORK assignment in env wins.
func ResolveNetworkAllowed(env []string) bool {
	for i := len(env) - 1; i >= 0; i-- {
		if v, ok := strings.CutPrefix(env[i], "DANGIT_NO_NETWORK="); ok {
			return !envTruthyString(v)
		}
	}
	return true
}

func envTruthyString(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "0", "false", "no", "off":
		return false
	default:
		return true
	}
}
