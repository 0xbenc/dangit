// Package scan discovers Git repositories under a directory tree and reports
// which ones have unfinished work — uncommitted changes, commits not yet pushed
// to their upstream, or commits waiting to be pulled. It is a faithful Go port
// of the bash-zoo `forgit` sweeper, with the per-repo remote checks run
// concurrently instead of serially.
package scan

import (
	"sort"
	"strconv"
)

// Ahead/Behind states are string-coded to mirror forgit's report vocabulary so
// the text output reads the same. Numeric states are decimal counts.
const (
	// StateNone means "in sync on this axis".
	StateNone = "0"
	// AheadNoUpstream marks a branch with local commits but no configured
	// upstream to push them to.
	AheadNoUpstream = "no-upstream"
	// BehindUnknown marks a repo known to be behind by an unknown amount
	// (the remote head is not present locally and could not be counted).
	BehindUnknown = "unknown"
	// BehindStale marks a repo whose remote could not be reached (timeout or
	// --no-network), so its behind status is uncertain.
	BehindStale = "stale"
)

// Result is the inspection outcome for a single working-tree repository.
type Result struct {
	// Path is the display path, relative to the scan root when possible
	// (".") for the root repo itself).
	Path string `json:"path"`
	// AbsPath is the absolute path to the repository working tree.
	AbsPath string `json:"abs_path"`
	// Branch is the current branch, or "detached@<short-sha>" / "detached".
	Branch string `json:"branch"`
	// HasChanges is true when `git status --porcelain` reported anything.
	HasChanges bool `json:"changes"`
	// Ahead is "0", a decimal count, or AheadNoUpstream.
	Ahead string `json:"ahead"`
	// Behind is "0", a decimal count, BehindUnknown, or BehindStale.
	Behind string `json:"behind"`
	// Err is a non-empty inspection error message, if any.
	Err string `json:"error,omitempty"`
}

// AheadCount returns the numeric ahead count, or 0 for non-numeric states.
func (r Result) AheadCount() int { return atoiSafe(r.Ahead) }

// BehindCount returns the numeric behind count, or 0 for non-numeric states.
func (r Result) BehindCount() int { return atoiSafe(r.Behind) }

// NeedsAttention reports whether the repo is flagged for any reason.
func (r Result) NeedsAttention() bool {
	if r.HasChanges {
		return true
	}
	if r.Ahead != "" && r.Ahead != StateNone {
		return true
	}
	if r.Behind != "" && r.Behind != StateNone {
		return true
	}
	return false
}

// Summary tallies a slice of results for the report footer.
type Summary struct {
	Total           int `json:"total"`
	Flagged         int `json:"flagged"`
	Changes         int `json:"changes"`
	Ahead           int `json:"ahead"`
	AheadNoUpstream int `json:"ahead_no_upstream"`
	Behind          int `json:"behind"`
	BehindUnknown   int `json:"behind_unknown"`
	BehindStale     int `json:"behind_stale"`
}

// Summarize tallies results into a Summary.
func Summarize(results []Result) Summary {
	var s Summary
	s.Total = len(results)
	for _, r := range results {
		if !r.NeedsAttention() {
			continue
		}
		s.Flagged++
		if r.HasChanges {
			s.Changes++
		}
		switch {
		case r.Ahead == AheadNoUpstream:
			s.Ahead++
			s.AheadNoUpstream++
		case r.Ahead != "" && r.Ahead != StateNone:
			s.Ahead++
		}
		switch {
		case r.Behind == BehindUnknown:
			s.Behind++
			s.BehindUnknown++
		case r.Behind == BehindStale:
			s.Behind++
			s.BehindStale++
		case r.Behind != "" && r.Behind != StateNone:
			s.Behind++
		}
	}
	return s
}

// Flagged returns only the results that need attention, sorted by path.
func Flagged(results []Result) []Result {
	out := make([]Result, 0, len(results))
	for _, r := range results {
		if r.NeedsAttention() {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func atoiSafe(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}
