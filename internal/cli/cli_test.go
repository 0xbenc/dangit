package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xbenc/dangit/internal/cli"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := []string{}
	if dir != "" {
		full = append(full, "-C", dir)
	}
	full = append(full, "-c", "init.defaultBranch=main", "-c", "commit.gpgsign=false")
	full = append(full, args...)
	cmd := exec.Command("git", full...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// dirtyFixture returns a work dir containing one repo with uncommitted changes.
func dirtyFixture(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	tmp := t.TempDir()
	origin := filepath.Join(tmp, "origin.git")
	git(t, "", "init", "--bare", origin)
	seed := filepath.Join(tmp, "seed")
	git(t, "", "clone", "-q", origin, seed)
	if err := os.WriteFile(filepath.Join(seed, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, seed, "add", "-A")
	git(t, seed, "commit", "-qm", "init")
	git(t, seed, "push", "-q", "origin", "main")

	work := filepath.Join(tmp, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(work, "repo")
	git(t, "", "clone", "-q", origin, repo)
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a\nchanged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return work
}

func setupEnv(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_AUTHOR_NAME", "dangit-test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@dangit.local")
	t.Setenv("GIT_COMMITTER_NAME", "dangit-test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@dangit.local")
	t.Setenv("NO_COLOR", "1")
}

func run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := cli.Run(args, &out, &errb, cli.BuildInfo{Version: "test"})
	return code, out.String(), errb.String()
}

func TestScanFlaggedExit1(t *testing.T) {
	setupEnv(t)
	work := dirtyFixture(t)
	code, out, _ := run(t, "scan", work)
	if code != 1 {
		t.Fatalf("exit = %d, want 1\n%s", code, out)
	}
	if !strings.Contains(out, "Needs attention") {
		t.Errorf("expected a needs-attention report, got:\n%s", out)
	}
}

func TestScanJSONEnvelope(t *testing.T) {
	setupEnv(t)
	work := dirtyFixture(t)
	code, out, _ := run(t, "scan", "--json", work)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	var env struct {
		Summary struct {
			Total   int `json:"total"`
			Flagged int `json:"flagged"`
			Changes int `json:"changes"`
		} `json:"summary"`
		Repos []struct {
			Path    string `json:"path"`
			Changes bool   `json:"changes"`
		} `json:"repos"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if env.Summary.Flagged != 1 || env.Summary.Changes != 1 {
		t.Errorf("summary = %+v, want flagged=1 changes=1", env.Summary)
	}
	if len(env.Repos) != 1 || !env.Repos[0].Changes {
		t.Errorf("repos = %+v, want one changed repo", env.Repos)
	}
}

func TestScanCleanExit0(t *testing.T) {
	setupEnv(t)
	tmp := t.TempDir() // empty: no repos to flag
	code, out, _ := run(t, "scan", tmp)
	if code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, out)
	}
}

func TestInvalidTimeoutExit2(t *testing.T) {
	setupEnv(t)
	code, _, errb := run(t, "scan", "--timeout-secs", "nope")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb, "invalid timeout") {
		t.Errorf("stderr = %q", errb)
	}
}

func TestResolveNoNetworkRefused(t *testing.T) {
	setupEnv(t)
	work := dirtyFixture(t)
	code, _, errb := run(t, "resolve", "--no-network", "--yes", work)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb, "network") {
		t.Errorf("stderr = %q", errb)
	}
}

func TestVersion(t *testing.T) {
	code, out, _ := run(t, "version")
	if code != 0 || !strings.Contains(out, "dangit") {
		t.Errorf("version output = %q (code %d)", out, code)
	}
}
