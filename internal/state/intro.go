// Package state holds dangit's small per-user runtime bookkeeping (separate from
// the theme config), persisted as JSON under the state directory.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/0xbenc/dangit/internal/fsutil"
	"github.com/0xbenc/termtheme"
)

// IntroSchemaVersion is stamped into intro.json so a future reader can detect a
// file written by a newer dangit.
const IntroSchemaVersion = 1

// IntroState is the singleton persisted at <stateDir>/intro.json. It records the
// last startup-intro version the user has seen, so the animation plays only once
// per release. It lives in the state directory (not the theme config) because it
// is derived runtime bookkeeping, not user-authored configuration.
type IntroState struct {
	SchemaVersion        int    `json:"schema_version"`
	LastSeenIntroVersion string `json:"last_seen_intro_version"`
}

// ResolveDir returns dangit's per-user state directory: $DANGIT_STATE_DIR, else
// the OS-appropriate default (macOS Application Support, else $XDG_STATE_HOME or
// ~/.local/state), each under a "dangit" subdir.
func ResolveDir(env []string) (string, error) {
	values := termtheme.EnvMap(env)
	if dir := strings.TrimSpace(values["DANGIT_STATE_DIR"]); dir != "" {
		return filepath.Clean(dir), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "dangit"), nil
	}
	if xdg := strings.TrimSpace(values["XDG_STATE_HOME"]); xdg != "" {
		return filepath.Join(xdg, "dangit"), nil
	}
	return filepath.Join(home, ".local", "state", "dangit"), nil
}

// IntroPath returns the on-disk location of the intro singleton.
func IntroPath(dir string) string {
	return filepath.Join(filepath.Clean(dir), "intro.json")
}

// ReadIntro loads the intro singleton. A missing file is not an error: it returns
// the zero value (no version seen yet) so a first run plays the intro. A
// present-but-corrupt file returns an error.
func ReadIntro(dir string) (IntroState, error) {
	data, err := os.ReadFile(IntroPath(dir))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return IntroState{}, nil
		}
		return IntroState{}, fmt.Errorf("read intro state: %w", err)
	}
	var intro IntroState
	if err := json.Unmarshal(data, &intro); err != nil {
		return IntroState{}, fmt.Errorf("parse intro state: %w", err)
	}
	return intro, nil
}

// WriteIntro persists the intro singleton atomically with 0o600 permissions,
// stamping the current schema version when unset.
func WriteIntro(dir string, intro IntroState) error {
	if intro.SchemaVersion <= 0 {
		intro.SchemaVersion = IntroSchemaVersion
	}
	data, err := json.MarshalIndent(intro, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal intro state: %w", err)
	}
	data = append(data, '\n')
	if _, err := fsutil.AtomicWriteFile(IntroPath(dir), data, fsutil.WriteOptions{Mode: 0o600}); err != nil {
		return fmt.Errorf("write intro state: %w", err)
	}
	return nil
}
