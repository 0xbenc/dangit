package scan

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// gitRunner runs git subcommands for a scan, carrying the network policy and
// the environment used for remote operations.
type gitRunner struct {
	baseEnv   []string
	timeout   time.Duration
	noNetwork bool
}

func newGitRunner(env []string, timeout time.Duration, noNetwork bool) gitRunner {
	if env == nil {
		env = os.Environ()
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return gitRunner{baseEnv: env, timeout: timeout, noNetwork: noNetwork}
}

// git runs `git -C dir args...` and returns trimmed stdout. A non-zero exit is
// returned as an error; stdout is still returned (some callers only care
// whether the command succeeded).
func (g gitRunner) git(ctx context.Context, dir string, args ...string) (string, error) {
	return g.gitEnv(ctx, dir, g.baseEnv, args...)
}

// gitNet runs a remote-touching git command under a fresh per-call timeout and
// with prompts disabled / SSH in batch mode so it fails fast instead of hanging
// on credential or host-key prompts. Mirrors forgit's run_with_timeout + the
// GIT_TERMINAL_PROMPT / GIT_SSH_COMMAND guards.
func (g gitRunner) gitNet(ctx context.Context, dir string, args ...string) (string, error) {
	netCtx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()
	secs := int(g.timeout / time.Second)
	if secs < 1 {
		secs = 1
	}
	env := append([]string(nil), g.baseEnv...)
	env = append(env,
		"GIT_TERMINAL_PROMPT=0",
		fmt.Sprintf("GIT_SSH_COMMAND=ssh -o BatchMode=yes -o ConnectTimeout=%d -o ConnectionAttempts=1", secs),
	)
	return g.gitEnv(netCtx, dir, env, args...)
}

func (g gitRunner) gitEnv(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	full := append([]string{"-C", dir}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out := strings.TrimRight(stdout.String(), "\n")
	if err != nil {
		// Prefer git's own message (first line of stderr) over the raw exit
		// status, so callers can surface something like "Author identity
		// unknown" instead of the whole command line.
		if detail := firstLine(stderr.String()); detail != "" {
			return out, fmt.Errorf("%s", detail)
		}
		return out, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

// inspect runs the full per-repo check pipeline. ok is false when the path is
// not a working tree (e.g. a bare repo) and should be dropped from results.
func (g gitRunner) inspect(ctx context.Context, repoDir string) (res Result, ok bool) {
	if out, err := g.git(ctx, repoDir, "rev-parse", "--is-inside-work-tree"); err != nil || strings.TrimSpace(out) != "true" {
		return Result{}, false
	}

	res = Result{AbsPath: repoDir, Ahead: StateNone, Behind: StateNone}

	// Uncommitted changes (staged, unstaged, or untracked).
	if status, err := g.git(ctx, repoDir, "status", "--porcelain"); err == nil && strings.TrimSpace(status) != "" {
		res.HasChanges = true
	}

	// Current branch; fall back to a detached-HEAD display.
	branch, branchErr := g.git(ctx, repoDir, "symbolic-ref", "--short", "-q", "HEAD")
	branch = sanitizeLine(branch)
	if branchErr != nil || branch == "" {
		res.Branch = detachedDisplay(g.git(ctx, repoDir, "rev-parse", "--short", "HEAD"))
		// Detached HEAD: no branch upstream to compare against.
		return res, true
	}
	res.Branch = branch

	g.checkUpstream(ctx, repoDir, branch, &res)
	return res, true
}

// checkUpstream resolves the branch's upstream and fills Ahead/Behind.
func (g gitRunner) checkUpstream(ctx context.Context, repoDir, branch string, res *Result) {
	remote, _ := g.git(ctx, repoDir, "config", "--get", "branch."+branch+".remote")
	merge, _ := g.git(ctx, repoDir, "config", "--get", "branch."+branch+".merge")
	remote = strings.TrimSpace(remote)
	merge = strings.TrimSpace(merge)

	if remote == "" || remote == "." || merge == "" {
		// No tracking upstream. If the repo has any commit, its work is
		// effectively "ahead" of a remote that does not exist yet.
		if _, err := g.git(ctx, repoDir, "rev-parse", "HEAD"); err == nil {
			res.Ahead = AheadNoUpstream
		}
		return
	}

	if g.noNetwork {
		g.localFallback(ctx, repoDir, res, BehindStale)
		return
	}

	out, err := g.gitNet(ctx, repoDir, "ls-remote", "--exit-code", remote, merge)
	if err != nil {
		// Timeout / unreachable / auth failure: count local-only commits and
		// mark the behind side as stale (uncertain), like forgit.
		g.localFallback(ctx, repoDir, res, BehindStale)
		return
	}
	remoteSHA := firstField(out)
	if remoteSHA == "" {
		g.localFallback(ctx, repoDir, res, BehindStale)
		return
	}

	// If we already have the remote commit locally we can count both sides.
	if _, err := g.git(ctx, repoDir, "cat-file", "-e", remoteSHA+"^{commit}"); err == nil {
		if counts, err := g.git(ctx, repoDir, "rev-list", "--left-right", "--count", "HEAD..."+remoteSHA); err == nil {
			ahead, behind := splitCounts(counts)
			res.Ahead = numericState(ahead)
			res.Behind = numericState(behind)
			return
		}
	}

	// Remote head not present locally: count what we can from the tracking ref
	// and mark behind as unknown.
	g.localFallback(ctx, repoDir, res, BehindUnknown)
}

// localFallback counts commits ahead of the local tracking ref (@{u}) and sets
// Behind to the supplied uncertain state. Used when the remote head can't be
// compared directly.
func (g gitRunner) localFallback(ctx context.Context, repoDir string, res *Result, behind string) {
	res.Behind = behind
	if _, err := g.git(ctx, repoDir, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); err != nil {
		return
	}
	if count, err := g.git(ctx, repoDir, "rev-list", "--count", "@{u}..HEAD"); err == nil {
		res.Ahead = numericState(count)
	}
}

func detachedDisplay(short string, err error) string {
	short = sanitizeLine(short)
	if err != nil || short == "" {
		return "detached"
	}
	return "detached@" + short
}

func numericState(s string) string {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n <= 0 {
		return StateNone
	}
	return strconv.Itoa(n)
}

func splitCounts(s string) (ahead, behind string) {
	fields := strings.Fields(strings.TrimSpace(s))
	if len(fields) >= 2 {
		return fields[0], fields[1]
	}
	return "0", "0"
}

func firstField(s string) string {
	for _, line := range strings.Split(s, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 {
			return fields[0]
		}
	}
	return ""
}

func sanitizeLine(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "")
	return strings.TrimSpace(s)
}
