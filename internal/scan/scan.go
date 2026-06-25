package scan

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultTimeout is the per-repo remote-check timeout when none is given.
const DefaultTimeout = 10 * time.Second

// Options configures a scan.
type Options struct {
	// Root is the directory to scan (defaults to "." if empty).
	Root string
	// Timeout bounds each per-repo remote check.
	Timeout time.Duration
	// NoNetwork skips remote checks entirely (behind status becomes stale).
	NoNetwork bool
	// Concurrency caps simultaneous repo inspections (defaults to a sensible value).
	Concurrency int
	// Env is the environment for git invocations (defaults to os.Environ()).
	Env []string
	// Progress, if set, is called as each repo finishes with the running done
	// count, the total, and the just-finished repo's display path. It may be
	// called from multiple goroutines, but calls are serialized.
	Progress func(done, total int, current string)
}

// Discover walks root and returns every working-tree-bearing repository
// directory (the parent of a `.git` entry). Nested repositories are included,
// matching forgit's `find -name .git` behavior.
func Discover(root string) ([]string, error) {
	var repos []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Unreadable directory: skip it rather than aborting the whole walk.
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.Name() == ".git" {
			repos = append(repos, filepath.Dir(path))
			if d.IsDir() {
				return fs.SkipDir // never descend into .git internals
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(repos)
	return repos, nil
}

// Scan discovers repositories under opts.Root and inspects them concurrently,
// returning one Result per working-tree repo (bare repos are dropped), sorted
// by display path.
func Scan(ctx context.Context, opts Options) ([]Result, error) {
	root := opts.Root
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	repos, err := Discover(root)
	if err != nil {
		return nil, err
	}

	runner := newGitRunner(opts.Env, timeoutOrDefault(opts.Timeout), opts.NoNetwork)

	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = defaultConcurrency()
	}
	if concurrency > len(repos) {
		concurrency = len(repos)
	}

	total := len(repos)
	results := make([]Result, total)
	keep := make([]bool, total)

	var done int64
	var progressMu sync.Mutex
	report := func(repoDisplay string) {
		if opts.Progress == nil {
			return
		}
		n := int(atomic.AddInt64(&done, 1))
		progressMu.Lock()
		opts.Progress(n, total, repoDisplay)
		progressMu.Unlock()
	}

	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if ctx.Err() != nil {
					return
				}
				res, ok := runner.inspect(ctx, repos[i])
				if ok {
					res.Path = displayPath(absRoot, repos[i])
					results[i] = res
					keep[i] = true
				}
				report(displayPath(absRoot, repos[i]))
			}
		}()
	}
	for i := range repos {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return nil, ctx.Err()
		case jobs <- i:
		}
	}
	close(jobs)
	wg.Wait()

	out := make([]Result, 0, total)
	for i := range results {
		if keep[i] {
			out = append(out, results[i])
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// InspectRepo re-inspects a single working-tree repo, for refreshing one row
// after an interactive action. ok is false for a non-working-tree path. The
// returned Result.Path is the absolute path; callers that track a display path
// should preserve their own.
func InspectRepo(ctx context.Context, dir string, opts Options) (Result, bool) {
	runner := newGitRunner(opts.Env, timeoutOrDefault(opts.Timeout), opts.NoNetwork)
	res, ok := runner.inspect(ctx, dir)
	if ok {
		res.Path = dir
	}
	return res, ok
}

func timeoutOrDefault(d time.Duration) time.Duration {
	if d <= 0 {
		return DefaultTimeout
	}
	return d
}

func defaultConcurrency() int {
	// Remote checks are network-bound, so oversubscribe CPUs, but keep a lid on
	// it to avoid spawning a flood of ssh connections.
	n := runtime.GOMAXPROCS(0) * 4
	if n < 4 {
		n = 4
	}
	if n > 32 {
		n = 32
	}
	return n
}

// displayPath renders repo relative to root: "." for the root repo, the
// relative path when repo is under root, else the absolute path.
func displayPath(absRoot, repo string) string {
	rel, err := filepath.Rel(absRoot, repo)
	if err != nil || rel == ".." || filepath.IsAbs(rel) || hasDotDotPrefix(rel) {
		return repo
	}
	return rel
}

func hasDotDotPrefix(rel string) bool {
	return rel == ".." || (len(rel) >= 3 && rel[0] == '.' && rel[1] == '.' && os.IsPathSeparator(rel[2]))
}
