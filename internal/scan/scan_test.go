package scan_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xbenc/dangit/internal/scan"
)

// gitEnv returns a deterministic, isolated environment for git: no global/system
// config, a fixed identity, and no credential prompts. Overridden keys are
// stripped from the inherited environment so the child sees a single value.
func gitEnv() []string {
	overrides := map[string]string{
		"GIT_CONFIG_GLOBAL":   os.DevNull,
		"GIT_CONFIG_SYSTEM":   os.DevNull,
		"GIT_AUTHOR_NAME":     "dangit-test",
		"GIT_AUTHOR_EMAIL":    "test@dangit.local",
		"GIT_COMMITTER_NAME":  "dangit-test",
		"GIT_COMMITTER_EMAIL": "test@dangit.local",
		"GIT_TERMINAL_PROMPT": "0",
	}
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if _, ok := overrides[k]; !ok {
			env = append(env, kv)
		}
	}
	for k, v := range overrides {
		env = append(env, k+"="+v)
	}
	return env
}

// git runs a git command from dir (use "" for none) and fails the test on error.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := []string{}
	if dir != "" {
		full = append(full, "-C", dir)
	}
	full = append(full, "-c", "init.defaultBranch=main", "-c", "commit.gpgsign=false")
	full = append(full, args...)
	cmd := exec.Command("git", full...)
	cmd.Env = gitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fixture builds a tree of repositories in known states and returns the work
// directory to scan plus the origin path.
type fixture struct {
	work   string
	origin string
}

func buildFixture(t *testing.T) fixture {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	tmp := t.TempDir()
	work := filepath.Join(tmp, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	origin := filepath.Join(tmp, "origin.git")
	git(t, "", "init", "--bare", origin)

	// Seed origin with commit A.
	seed := filepath.Join(tmp, "seed")
	git(t, "", "clone", "-q", origin, seed)
	write(t, filepath.Join(seed, "a.txt"), "A\n")
	git(t, seed, "add", "-A")
	git(t, seed, "commit", "-qm", "A")
	git(t, seed, "push", "-q", "origin", "main")

	// behind: clone at A, then origin advances to B, then fetch (so the commit
	// is local) without merging → numerically behind by 1.
	behind := filepath.Join(work, "behind")
	git(t, "", "clone", "-q", origin, behind)
	write(t, filepath.Join(seed, "a.txt"), "A\nB\n")
	git(t, seed, "commit", "-aqm", "B")
	git(t, seed, "push", "-q", "origin", "main")
	git(t, behind, "fetch", "-q")

	// clean: clone at B → in sync.
	git(t, "", "clone", "-q", origin, filepath.Join(work, "clean"))

	// dirty: clone at B, modify a tracked file (uncommitted).
	dirty := filepath.Join(work, "dirty")
	git(t, "", "clone", "-q", origin, dirty)
	write(t, filepath.Join(dirty, "a.txt"), "A\nB\nlocal\n")

	// ahead: clone at B, add a new committed-but-unpushed file.
	ahead := filepath.Join(work, "ahead")
	git(t, "", "clone", "-q", origin, ahead)
	write(t, filepath.Join(ahead, "c.txt"), "C\n")
	git(t, ahead, "add", "-A")
	git(t, ahead, "commit", "-qm", "local c")

	// noupstream: standalone repo with a commit and no remote.
	noup := filepath.Join(work, "noupstream")
	git(t, "", "init", "-q", noup)
	write(t, filepath.Join(noup, "n.txt"), "N\n")
	git(t, noup, "add", "-A")
	git(t, noup, "commit", "-qm", "solo")

	return fixture{work: work, origin: origin}
}

func scanMap(t *testing.T, fx fixture, noNetwork bool) map[string]scan.Result {
	t.Helper()
	results, err := scan.Scan(context.Background(), scan.Options{
		Root:      fx.work,
		Env:       gitEnv(),
		NoNetwork: noNetwork,
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	byPath := make(map[string]scan.Result, len(results))
	for _, r := range results {
		byPath[r.Path] = r
	}
	return byPath
}

func TestDiscoverCountsAllRepos(t *testing.T) {
	fx := buildFixture(t)
	repos, err := scan.Discover(fx.work)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 5 {
		t.Fatalf("expected 5 repos, got %d: %v", len(repos), repos)
	}
}

func TestScanStates(t *testing.T) {
	fx := buildFixture(t)
	m := scanMap(t, fx, false)

	if c, ok := m["clean"]; !ok {
		t.Errorf("clean repo missing from results")
	} else if c.NeedsAttention() {
		t.Errorf("clean repo should not need attention, got %+v", c)
	}

	dirty, ok := m["dirty"]
	if !ok || !dirty.HasChanges {
		t.Errorf("dirty should have changes: %+v", dirty)
	}

	ahead := m["ahead"]
	if ahead.Ahead != "1" {
		t.Errorf("ahead.Ahead = %q, want 1", ahead.Ahead)
	}

	behind := m["behind"]
	if behind.Behind != "1" {
		t.Errorf("behind.Behind = %q, want 1", behind.Behind)
	}

	noup := m["noupstream"]
	if noup.Ahead != scan.AheadNoUpstream {
		t.Errorf("noupstream.Ahead = %q, want %q", noup.Ahead, scan.AheadNoUpstream)
	}
}

func TestScanNoNetworkMarksStale(t *testing.T) {
	fx := buildFixture(t)
	m := scanMap(t, fx, true)
	// A repo with an upstream becomes stale (remote not consulted) and thus flagged.
	r, ok := m["clean"]
	if !ok {
		t.Fatalf("clean repo should be flagged as stale in no-network mode")
	}
	if r.Behind != scan.BehindStale {
		t.Errorf("clean.Behind = %q, want %q", r.Behind, scan.BehindStale)
	}
}

func TestSummarize(t *testing.T) {
	fx := buildFixture(t)
	results, err := scan.Scan(context.Background(), scan.Options{Root: fx.work, Env: gitEnv()})
	if err != nil {
		t.Fatal(err)
	}
	s := scan.Summarize(results)
	if s.Total != 5 {
		t.Errorf("Total = %d, want 5", s.Total)
	}
	if s.Flagged != 4 {
		t.Errorf("Flagged = %d, want 4", s.Flagged)
	}
	if s.Changes != 1 {
		t.Errorf("Changes = %d, want 1", s.Changes)
	}
	if s.AheadNoUpstream != 1 {
		t.Errorf("AheadNoUpstream = %d, want 1", s.AheadNoUpstream)
	}
}

func TestPlanResolveDirty(t *testing.T) {
	fx := buildFixture(t)
	plan, err := scan.PlanResolve(context.Background(), filepath.Join(fx.work, "dirty"), "", gitEnv())
	if err != nil {
		t.Fatal(err)
	}
	if !plan.WillCommit {
		t.Errorf("expected WillCommit for dirty repo")
	}
	if plan.CommitMsg == "" {
		t.Errorf("expected a generated commit message")
	}
	if !plan.WillPush {
		t.Errorf("expected WillPush (dirty has an upstream)")
	}
}

func TestResolveSuccess(t *testing.T) {
	fx := buildFixture(t)
	// A fresh clone with a non-conflicting new file resolves cleanly.
	repo := filepath.Join(fx.work, "resolveme")
	git(t, "", "clone", "-q", fx.origin, repo)
	write(t, filepath.Join(repo, "unique.txt"), "u\n")

	res := scan.Resolve(context.Background(), repo, scan.ResolveOptions{Env: gitEnv()})
	if res.Err != nil {
		t.Fatalf("resolve failed: %v", res.Err)
	}
	if !res.Committed || !res.Pulled || !res.Pushed {
		t.Fatalf("expected committed+pulled+pushed, got %+v", res)
	}
	// The repo is now clean and in sync.
	r, ok := scan.InspectRepo(context.Background(), repo, scan.Options{Env: gitEnv()})
	if !ok || r.NeedsAttention() {
		t.Errorf("repo should be clean after resolve: %+v", r)
	}
	// Origin received the dangit commit.
	log := git(t, fx.origin, "log", "--oneline")
	if !strings.Contains(log, "dangit:") {
		t.Errorf("origin missing dangit commit:\n%s", log)
	}
}

func TestResolveConflictAborts(t *testing.T) {
	fx := buildFixture(t)
	// Clone, then make a local change that will conflict with a new origin commit.
	repo := filepath.Join(fx.work, "conflict")
	git(t, "", "clone", "-q", fx.origin, repo)
	write(t, filepath.Join(repo, "a.txt"), "A\nB\nMINE\n")

	// Advance origin with a conflicting change on the same lines.
	seed2 := filepath.Join(t.TempDir(), "seed2")
	git(t, "", "clone", "-q", fx.origin, seed2)
	write(t, filepath.Join(seed2, "a.txt"), "A\nB\nTHEIRS\n")
	git(t, seed2, "commit", "-aqm", "theirs")
	git(t, seed2, "push", "-q", "origin", "main")

	res := scan.Resolve(context.Background(), repo, scan.ResolveOptions{Env: gitEnv()})
	if res.Err == nil {
		t.Fatalf("expected a conflict error, got success: %+v", res)
	}
	if !res.Committed {
		t.Errorf("local change should still have been committed before the failed pull")
	}
	// The rebase must have been aborted, leaving a clean working tree (no
	// half-finished rebase state).
	status := git(t, repo, "status", "--porcelain")
	if strings.TrimSpace(status) != "" {
		t.Errorf("working tree should be clean after aborted rebase, got:\n%s", status)
	}
	if _, err := os.Stat(filepath.Join(repo, ".git", "rebase-merge")); !os.IsNotExist(err) {
		t.Errorf("rebase state should be gone after abort")
	}
}
